package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/tethux/tethux/storage"
	storageerrs "github.com/tethux/tethux/storage/errs"
)

// DefaultName is the provider name used when no custom name is supplied.
const DefaultName storage.ProviderName = "local"

// Provider stores objects beneath a local filesystem root.
type Provider struct {
	name storage.ProviderName
	root string

	mu    sync.Mutex
	dirty map[storage.PreparedID]bool

	nextSubID   uint64
	subscribers map[uint64]chan storage.Event
}

var (
	_ storage.Provider      = (*Provider)(nil)
	_ storage.Manager       = (*Provider)(nil)
	_ storage.AsyncProvider = (*Provider)(nil)
	_ storage.EventSource   = (*Provider)(nil)
)

// Option configures a Provider.
type Option func(*Provider)

// WithName sets the provider name advertised by Provider.Name.
func WithName(name storage.ProviderName) Option {
	return func(p *Provider) {
		p.name = name
	}
}

// New creates a local provider rooted at root, creating the directory if needed.
func New(root string, opts ...Option) (*Provider, error) {
	if root == "" {
		return nil, storageerrs.New(string(DefaultName), storageerrs.ErrInvalidOptions, "root is empty")
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, storageerrs.Wrap(string(DefaultName), storageerrs.ErrInvalidOptions, "root", err)
	}

	if err := os.MkdirAll(abs, 0o750); err != nil {
		return nil, storageerrs.Wrap(string(DefaultName), storageerrs.ErrCreate, abs, err)
	}

	p := &Provider{
		name:        DefaultName,
		root:        abs,
		dirty:       make(map[storage.PreparedID]bool),
		subscribers: make(map[uint64]chan storage.Event),
	}

	for _, opt := range opts {
		opt(p)
	}

	if p.name == "" {
		return nil, storageerrs.New(string(DefaultName), storageerrs.ErrInvalidOptions, "provider name is empty")
	}

	return p, nil
}

// Name returns the provider's configured name.
func (p *Provider) Name() storage.ProviderName {
	return p.name
}

func (p *Provider) Info() storage.ProviderInfo {
	return storage.ProviderInfo{
		Name: p.name,
		Capabilities: storage.Capabilities{
			AtomicReplace:    true,
			AtomicMove:       true,
			ConditionalWrite: false,
		},
	}
}

// Root returns the provider's absolute filesystem root.
func (p *Provider) Root() string {
	return p.root
}

func (p *Provider) Stat(
	ctx context.Context,
	ref storage.Ref,
) (*storage.ObjectInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	path, err := p.pathFor(ref)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, storageerrs.Wrap(
				string(p.name),
				storageerrs.ErrNotFound,
				ref.String(),
				err,
			)
		}

		return nil, storageerrs.Wrap(
			string(p.name),
			storageerrs.ErrStat,
			ref.String(),
			err,
		)
	}

	generation := storage.Generation("")
	if info.Mode().IsRegular() {
		generation = storage.Generation(
			fmt.Sprintf(
				"%d:%d",
				info.ModTime().UnixNano(),
				info.Size(),
			),
		)
	}

	return &storage.ObjectInfo{
		Ref:         ref,
		Type:        objectType(info),
		Size:        info.Size(),
		ModTime:     info.ModTime(),
		Generation:  generation,
		ContentType: mime.TypeByExtension(filepath.Ext(path)),
		Checksum:    nil,
		Metadata:    storage.Metadata{},
		Kind:        storage.ArtifactGeneric,
	}, nil
}

func (p *Provider) Open(
	ctx context.Context,
	ref storage.Ref,
) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	path, err := p.pathFor(ref)
	if err != nil {
		return nil, err
	}

	// #nosec G304 -- pathFor rejects absolute, escaping, and symlinked refs.
	f, err := os.Open(path)
	if err != nil {
		return nil, storageerrs.Wrap(string(p.name), storageerrs.ErrOpen, ref.String(), err)
	}

	return f, nil
}

func (p *Provider) Put(
	ctx context.Context,
	ref storage.Ref,
	r io.Reader,
	opts storage.PutOptions,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r == nil {
		return storageerrs.New(string(p.name), storageerrs.ErrInvalidOptions, "input reader is nil")
	}

	path, err := p.pathFor(ref)
	if err != nil {
		return err
	}

	if mkdirErr := os.MkdirAll(filepath.Dir(path), 0o750); mkdirErr != nil {
		return storageerrs.Wrap(string(p.name), storageerrs.ErrCreate, ref.String(), mkdirErr)
	}

	mode := fs.FileMode(opts.Mode)
	if mode == 0 {
		mode = 0o644
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".tethux-*")
	if err != nil {
		return storageerrs.Wrap(string(p.name), storageerrs.ErrCreate, ref.String(), err)
	}

	tmpPath := tmp.Name()

	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}

	if err := tmp.Chmod(mode); err != nil {
		cleanup()
		return storageerrs.Wrap(string(p.name), storageerrs.ErrPut, ref.String(), err)
	}

	if _, err := copyContext(ctx, tmp, r); err != nil {
		cleanup()
		return storageerrs.Wrap(string(p.name), storageerrs.ErrPut, ref.String(), err)
	}

	if err := tmp.Sync(); err != nil {
		cleanup()
		return storageerrs.Wrap(string(p.name), storageerrs.ErrPut, ref.String(), err)
	}

	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return storageerrs.Wrap(string(p.name), storageerrs.ErrPut, ref.String(), err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return storageerrs.Wrap(string(p.name), storageerrs.ErrPut, ref.String(), err)
	}

	return nil
}

func (p *Provider) Delete(
	ctx context.Context,
	ref storage.Ref,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	path, err := p.pathFor(ref)
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return storageerrs.Wrap(string(p.name), storageerrs.ErrDelete, ref.String(), err)
	}

	return nil
}

func (p *Provider) List(
	ctx context.Context,
	prefix storage.Ref,
	opts storage.ListOptions,
) ([]storage.ObjectInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	path, err := p.pathFor(prefix)
	if err != nil {
		return nil, err
	}

	if opts.Recursive {
		return p.listRecursive(ctx, prefix, path)
	}

	return p.listDirect(ctx, prefix, path)
}

func (p *Provider) listDirect(
	ctx context.Context,
	prefix storage.Ref,
	path string,
) ([]storage.ObjectInfo, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, storageerrs.Wrap(
			string(p.name),
			storageerrs.ErrList,
			prefix.String(),
			err,
		)
	}

	out := make([]storage.ObjectInfo, 0, len(entries))

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		info, err := entry.Info()
		if err != nil {
			return nil, storageerrs.Wrap(
				string(p.name),
				storageerrs.ErrList,
				entry.Name(),
				err,
			)
		}

		key := filepath.ToSlash(
			filepath.Join(string(prefix.Key), entry.Name()),
		)

		out = append(out, objectInfoFromFileInfo(
			p.name,
			storage.Key(key),
			info,
		))
	}

	return out, nil
}

func (p *Provider) listRecursive(
	ctx context.Context,
	prefix storage.Ref,
	root string,
) ([]storage.ObjectInfo, error) {
	var out []storage.ObjectInfo

	err := filepath.WalkDir(
		root,
		func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}

			if err := ctx.Err(); err != nil {
				return err
			}

			if path == root {
				return nil
			}

			info, err := entry.Info()
			if err != nil {
				return err
			}

			rel, err := filepath.Rel(p.root, path)
			if err != nil {
				return err
			}

			out = append(out, objectInfoFromFileInfo(
				p.name,
				storage.Key(filepath.ToSlash(rel)),
				info,
			))

			return nil
		},
	)
	if err != nil {
		return nil, storageerrs.Wrap(
			string(p.name),
			storageerrs.ErrList,
			prefix.String(),
			err,
		)
	}

	return out, nil
}

func objectInfoFromFileInfo(
	provider storage.ProviderName,
	key storage.Key,
	info fs.FileInfo,
) storage.ObjectInfo {
	generation := storage.Generation("")

	if info.Mode().IsRegular() {
		generation = storage.Generation(
			fmt.Sprintf(
				"%d:%d",
				info.ModTime().UnixNano(),
				info.Size(),
			),
		)
	}

	return storage.ObjectInfo{
		Ref: storage.Ref{
			Provider: provider,
			Key:      key,
		},
		Type:       objectType(info),
		Size:       info.Size(),
		ModTime:    info.ModTime(),
		Generation: generation,
	}
}

func (p *Provider) Prepare(
	ctx context.Context,
	req storage.PrepareRequest, //nolint:gocritic // part of the public storage.Manager contract
) (*storage.Prepared, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	mode := req.Mode
	if mode == "" {
		mode = storage.PrepareDirect
	}

	if mode != storage.PrepareDirect {
		return nil, storageerrs.New(
			string(p.name),
			storageerrs.ErrUnsupportedMode,
			string(mode),
		)
	}

	path, err := p.pathFor(req.Ref)
	if err != nil {
		return nil, err
	}

	info, statErr := os.Stat(path)
	if statErr != nil {
		if !errors.Is(statErr, fs.ErrNotExist) {
			return nil, storageerrs.Wrap(
				string(p.name),
				storageerrs.ErrStat,
				req.Ref.String(),
				statErr,
			)
		}

		if !req.Create {
			return nil, storageerrs.Wrap(
				string(p.name),
				storageerrs.ErrNotFound,
				req.Ref.String(),
				statErr,
			)
		}

		switch req.ResourceType {
		case storage.ResourceTypeDirectory:
			mkdirErr := os.MkdirAll(path, 0o750)
			if mkdirErr != nil {
				return nil, storageerrs.Wrap(
					string(p.name),
					storageerrs.ErrCreate,
					req.Ref.String(),
					mkdirErr,
				)
			}

		case storage.ResourceTypeFile:
			parentMkdirErr := os.MkdirAll(filepath.Dir(path), 0o750)
			if parentMkdirErr != nil {
				return nil, storageerrs.Wrap(
					string(p.name),
					storageerrs.ErrCreate,
					req.Ref.String(),
					parentMkdirErr,
				)
			}

			// #nosec G304 -- pathFor rejects absolute, escaping, and symlinked refs.
			file, openErr := os.OpenFile(
				path,
				os.O_CREATE|os.O_EXCL|os.O_WRONLY,
				0o600,
			)
			if openErr != nil {
				return nil, storageerrs.Wrap(
					string(p.name),
					storageerrs.ErrCreate,
					req.Ref.String(),
					openErr,
				)
			}

			closeErr := file.Close()
			if closeErr != nil {
				return nil, storageerrs.Wrap(
					string(p.name),
					storageerrs.ErrCreate,
					req.Ref.String(),
					closeErr,
				)
			}

		default:
			return nil, storageerrs.New(
				string(p.name),
				storageerrs.ErrInvalidResourceType,
				req.Ref.String(),
			)
		}

		info, err = os.Stat(path)
		if err != nil {
			return nil, storageerrs.Wrap(
				string(p.name),
				storageerrs.ErrStat,
				req.Ref.String(),
				err,
			)
		}
	}

	validationErr := validateResourceType(req.ResourceType, info)
	if validationErr != nil {
		return nil, storageerrs.Wrap(
			string(p.name),
			storageerrs.ErrInvalidResourceType,
			req.Ref.String(),
			validationErr,
		)
	}

	resourceType := req.ResourceType
	if resourceType == "" {
		switch {
		case info.Mode().IsRegular():
			resourceType = storage.ResourceTypeFile
		case info.IsDir():
			resourceType = storage.ResourceTypeDirectory
		default:
			return nil, storageerrs.New(
				string(p.name),
				storageerrs.ErrInvalidResourceType,
				req.Ref.String(),
			)
		}
	}

	objectInfo, err := p.Stat(ctx, req.Ref)
	if err != nil {
		return nil, err
	}

	prepared := &storage.Prepared{
		ID:             storage.PreparedID(uuid.NewString()),
		Ref:            req.Ref,
		BaseGeneration: objectInfo.Generation,
		NodeID:         req.NodeID,
		AccessMode:     req.AccessMode,
		Mode:           mode,
		ResourceType:   resourceType,
		Ownership:      storage.OwnershipExternal,
		Location: storage.RuntimeLocation{
			Kind:  storage.LocationPath,
			Value: path,
		},
	}

	p.emit(storage.Event{
		Type:       storage.EventPrepared,
		Time:       time.Now(),
		PreparedID: prepared.ID,
		NodeID:     prepared.NodeID,
		Ref:        refPtr(prepared.Ref),
		Generation: prepared.BaseGeneration,
	})

	return prepared, nil
}

func (p *Provider) PrepareAsync(
	ctx context.Context,
	req storage.PrepareRequest, //nolint:gocritic // part of the public storage.Manager contract
) (storage.PrepareOperation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	op := newOperation(ctx, storage.OperationPrepare, &req.Ref, nil)

	go func() {
		op.start()

		prepared, err := p.Prepare(op.context(), req)

		if prepared != nil {
			op.setPreparedID(prepared.ID)
		}

		op.setPrepared(prepared, err)
		op.finish(err)
	}()

	return op, nil
}

func (p *Provider) Events(
	ctx context.Context,
) (<-chan storage.Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	out := make(chan storage.Event, 16)
	sub := make(chan storage.Event, 16)

	p.mu.Lock()
	id := p.nextSubID
	p.nextSubID++
	p.subscribers[id] = sub
	p.mu.Unlock()

	go func() {
		defer close(out)

		defer func() {
			p.mu.Lock()
			delete(p.subscribers, id)
			p.mu.Unlock()
		}()

		for {
			select {
			case event := <-sub:
				select {
				case out <- event:
				case <-ctx.Done():
					return
				}

			case <-ctx.Done():
				return
			}
		}
	}()

	return out, nil
}

func (p *Provider) emit(event storage.Event) { //nolint:gocritic // storage.Event is the event-source contract
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, subscriber := range p.subscribers {
		select {
		case subscriber <- event:
		default:
		}
	}
}

func (p *Provider) MarkDirty(
	ctx context.Context,
	prepared *storage.Prepared,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if prepared == nil {
		return storageerrs.New(
			string(p.name),
			storageerrs.ErrInvalidOptions,
			"prepared storage is nil",
		)
	}

	if prepared.Ref.Provider != p.name {
		return storageerrs.New(
			string(p.name),
			storageerrs.ErrProviderMismatch,
			string(prepared.Ref.Provider),
		)
	}

	if prepared.AccessMode == storage.AccessReadOnly {
		return storageerrs.New(
			string(p.name),
			storageerrs.ErrInvalidOptions,
			"read-only prepared storage cannot be marked dirty",
		)
	}

	p.mu.Lock()
	alreadyDirty := p.dirty[prepared.ID]
	p.dirty[prepared.ID] = true
	p.mu.Unlock()

	if !alreadyDirty {
		p.emit(storage.Event{
			Type:       storage.EventDirty,
			Time:       time.Now(),
			PreparedID: prepared.ID,
			NodeID:     prepared.NodeID,
			Ref:        refPtr(prepared.Ref),
		})
	}

	return nil
}

func (p *Provider) Copy(
	ctx context.Context,
	src storage.Ref,
	dst storage.Ref,
	opts storage.CopyOptions,
) error {
	ctxErr := ctx.Err()
	if ctxErr != nil {
		return ctxErr
	}

	if src == dst {
		return storageerrs.New(
			string(p.name),
			storageerrs.ErrSourceEqualsDestination,
			src.String(),
		)
	}

	srcPath, pathErr := p.pathFor(src)
	if pathErr != nil {
		return pathErr
	}

	dstPath, pathErr := p.pathFor(dst)
	if pathErr != nil {
		return pathErr
	}

	srcInfo, statErr := os.Lstat(srcPath)
	if statErr != nil {
		if errors.Is(statErr, fs.ErrNotExist) {
			return storageerrs.Wrap(
				string(p.name),
				storageerrs.ErrNotFound,
				src.String(),
				statErr,
			)
		}

		return storageerrs.Wrap(
			string(p.name),
			storageerrs.ErrCopy,
			src.String(),
			statErr,
		)
	}

	if srcInfo.Mode()&os.ModeSymlink != 0 {
		return storageerrs.New(
			string(p.name),
			storageerrs.ErrCopy,
			src.String()+" -> "+dst.String(),
		)
	}

	dstInfo, dstStatErr := os.Lstat(dstPath)
	dstExists := dstStatErr == nil

	if dstStatErr != nil && !errors.Is(dstStatErr, fs.ErrNotExist) {
		return storageerrs.Wrap(
			string(p.name),
			storageerrs.ErrCopy,
			dst.String(),
			dstStatErr,
		)
	}

	if dstExists {
		if !opts.Overwrite {
			return storageerrs.New(
				string(p.name),
				storageerrs.ErrAlreadyExists,
				dst.String(),
			)
		}

		if srcInfo.IsDir() != dstInfo.IsDir() {
			return storageerrs.New(
				string(p.name),
				storageerrs.ErrCopy,
				src.String()+" -> "+dst.String(),
			)
		}

		if !srcInfo.IsDir() &&
			(!srcInfo.Mode().IsRegular() || !dstInfo.Mode().IsRegular()) {
			return storageerrs.New(
				string(p.name),
				storageerrs.ErrCopy,
				src.String()+" -> "+dst.String(),
			)
		}
	}

	copyErr := p.copyPath(
		ctx,
		srcPath,
		dstPath,
		opts.Overwrite,
	)
	if copyErr != nil {
		return storageerrs.Wrap(
			string(p.name),
			storageerrs.ErrCopy,
			src.String()+" -> "+dst.String(),
			copyErr,
		)
	}

	return nil
}

// CopyAsync starts a copy in the background and returns its operation handle.
func (p *Provider) CopyAsync(
	ctx context.Context,
	src storage.Ref,
	dst storage.Ref,
	opts storage.CopyOptions,
) (storage.OperationHandle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	op := newOperation(ctx, storage.OperationCopy, &src, &dst)
	go func() {
		op.start()
		op.finish(p.Copy(op.context(), src, dst, opts))
	}()
	return op, nil
}

func (p *Provider) Move(
	ctx context.Context,
	src storage.Ref,
	dst storage.Ref,
	opts storage.MoveOptions,
) error {
	ctxErr := ctx.Err()
	if ctxErr != nil {
		return ctxErr
	}

	if src == dst {
		return storageerrs.New(
			string(p.name),
			storageerrs.ErrSourceEqualsDestination,
			src.String(),
		)
	}

	srcPath, pathErr := p.pathFor(src)
	if pathErr != nil {
		return pathErr
	}

	dstPath, pathErr := p.pathFor(dst)
	if pathErr != nil {
		return pathErr
	}

	srcInfo, statErr := os.Lstat(srcPath)
	if statErr != nil {
		if errors.Is(statErr, fs.ErrNotExist) {
			return storageerrs.Wrap(
				string(p.name),
				storageerrs.ErrNotFound,
				src.String(),
				statErr,
			)
		}

		return storageerrs.Wrap(
			string(p.name),
			storageerrs.ErrMove,
			src.String(),
			statErr,
		)
	}

	if srcInfo.Mode()&os.ModeSymlink != 0 {
		return storageerrs.New(
			string(p.name),
			storageerrs.ErrMove,
			src.String()+" -> "+dst.String(),
		)
	}

	dstInfo, dstStatErr := os.Lstat(dstPath)
	dstExists := dstStatErr == nil

	if dstStatErr != nil && !errors.Is(dstStatErr, fs.ErrNotExist) {
		return storageerrs.Wrap(
			string(p.name),
			storageerrs.ErrMove,
			dst.String(),
			dstStatErr,
		)
	}

	if dstExists {
		if !opts.Overwrite {
			return storageerrs.New(
				string(p.name),
				storageerrs.ErrAlreadyExists,
				dst.String(),
			)
		}

		if srcInfo.IsDir() != dstInfo.IsDir() {
			return storageerrs.New(
				string(p.name),
				storageerrs.ErrMove,
				src.String()+" -> "+dst.String(),
			)
		}

		var removeErr error
		if dstInfo.IsDir() {
			removeErr = os.RemoveAll(dstPath)
		} else {
			removeErr = os.Remove(dstPath)
		}
		if removeErr != nil {
			return storageerrs.Wrap(
				string(p.name),
				storageerrs.ErrMove,
				dst.String(),
				removeErr,
			)
		}
	}

	mkdirErr := os.MkdirAll(filepath.Dir(dstPath), 0o750)
	if mkdirErr != nil {
		return storageerrs.Wrap(
			string(p.name),
			storageerrs.ErrMove,
			dst.String(),
			mkdirErr,
		)
	}

	renameErr := os.Rename(srcPath, dstPath)
	if renameErr != nil {
		return storageerrs.Wrap(
			string(p.name),
			storageerrs.ErrMove,
			src.String()+" -> "+dst.String(),
			renameErr,
		)
	}

	return nil
}

func validateResourceType(resourceType storage.ResourceType, info os.FileInfo) error {
	switch resourceType {
	case "":
		return nil
	case storage.ResourceTypeFile:
		if !info.Mode().IsRegular() {
			return storageerrs.New(string(DefaultName), storageerrs.ErrInvalidResourceType, "expected regular file")
		}
	case storage.ResourceTypeDirectory:
		if !info.IsDir() {
			return storageerrs.New(string(DefaultName), storageerrs.ErrInvalidResourceType, "expected directory")
		}
	default:
		return storageerrs.New(string(DefaultName), storageerrs.ErrInvalidResourceType, string(resourceType))
	}
	return nil
}

func (p *Provider) Release(
	ctx context.Context,
	prepared *storage.Prepared,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if prepared == nil {
		return storageerrs.New(
			string(p.name),
			storageerrs.ErrInvalidOptions,
			"prepared storage is nil",
		)
	}

	if prepared.Ref.Provider != p.name {
		return storageerrs.New(
			string(p.name),
			storageerrs.ErrProviderMismatch,
			string(prepared.Ref.Provider),
		)
	}

	p.mu.Lock()
	delete(p.dirty, prepared.ID)
	p.mu.Unlock()

	p.emit(storage.Event{
		Type:       storage.EventReleased,
		Time:       time.Now(),
		PreparedID: prepared.ID,
		NodeID:     prepared.NodeID,
		Ref:        refPtr(prepared.Ref),
	})

	return nil
}

func (p *Provider) ReleaseAsync(
	ctx context.Context,
	prepared *storage.Prepared,
) (storage.OperationHandle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if prepared == nil {
		return nil, storageerrs.New(
			string(p.name),
			storageerrs.ErrInvalidOptions,
			"prepared storage is nil",
		)
	}

	op := newOperation(
		ctx,
		storage.OperationRelease,
		&prepared.Ref,
		nil,
	)

	op.setPreparedID(prepared.ID)

	go func() {
		op.start()
		op.finish(p.Release(op.context(), prepared))
	}()

	return op, nil
}

func (p *Provider) pathFor(ref storage.Ref) (string, error) {
	if err := ref.Validate(); err != nil {
		return "", err
	}

	if ref.Provider != p.name {
		return "", storageerrs.New(string(p.name), storageerrs.ErrProviderMismatch, string(ref.Provider))
	}

	key := filepath.FromSlash(string(ref.Key))

	if filepath.IsAbs(key) {
		return "", storageerrs.New(string(p.name), storageerrs.ErrInvalidRef, string(ref.Key))
	}

	clean := filepath.Clean(key)

	if clean == "." || clean == "" {
		return "", storageerrs.New(string(p.name), storageerrs.ErrInvalidRef, string(ref.Key))
	}

	if clean == ".." ||
		strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", storageerrs.New(string(p.name), storageerrs.ErrInvalidRef, string(ref.Key))
	}

	full := filepath.Join(p.root, clean)

	rel, err := filepath.Rel(p.root, full)
	if err != nil {
		return "", storageerrs.Wrap(string(p.name), storageerrs.ErrInvalidRef, string(ref.Key), err)
	}

	if rel == ".." ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", storageerrs.New(string(p.name), storageerrs.ErrInvalidRef, string(ref.Key))
	}
	current := p.root
	for component := range strings.SplitSeq(clean, string(filepath.Separator)) {
		current = filepath.Join(current, component)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, fs.ErrNotExist) {
			break
		}
		if statErr != nil {
			return "", storageerrs.Wrap(string(p.name), storageerrs.ErrStat, current, statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", storageerrs.New(string(p.name), storageerrs.ErrInvalidRef, string(ref.Key))
		}
	}

	return full, nil
}

func objectType(info fs.FileInfo) storage.ObjectType {
	switch {
	case info.Mode().IsRegular():
		return storage.ObjectFile
	case info.IsDir():
		return storage.ObjectDir
	default:
		return storage.ObjectUnknown
	}
}

func copyContext(
	ctx context.Context,
	dst io.Writer,
	src io.Reader,
) (int64, error) {
	buf := make([]byte, 128*1024)

	var written int64

	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}

		n, readErr := src.Read(buf)
		if n > 0 {
			wn, writeErr := dst.Write(buf[:n])
			written += int64(wn)

			if writeErr != nil {
				return written, writeErr
			}
			if wn != n {
				return written, io.ErrShortWrite
			}
		}

		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return written, nil
			}
			return written, readErr
		}
	}
}

func (p *Provider) copyPath(
	ctx context.Context,
	srcPath string,
	dstPath string,
	overwrite bool,
) error {
	ctxErr := ctx.Err()
	if ctxErr != nil {
		return ctxErr
	}

	srcInfo, statErr := os.Lstat(srcPath)
	if statErr != nil {
		return statErr
	}

	if srcInfo.Mode()&os.ModeSymlink != 0 {
		return storageerrs.New(
			string(p.name),
			storageerrs.ErrCopy,
			srcPath,
		)
	}

	if srcInfo.IsDir() {
		dstInfo, dstStatErr := os.Lstat(dstPath)

		switch {
		case dstStatErr == nil:
			if !dstInfo.IsDir() {
				return storageerrs.New(
					string(p.name),
					storageerrs.ErrCopy,
					dstPath,
				)
			}

			if !overwrite {
				return storageerrs.New(
					string(p.name),
					storageerrs.ErrAlreadyExists,
					dstPath,
				)
			}

		case errors.Is(dstStatErr, fs.ErrNotExist):
			mkdirErr := os.MkdirAll(dstPath, srcInfo.Mode().Perm())
			if mkdirErr != nil {
				return mkdirErr
			}

		default:
			return dstStatErr
		}

		chmodErr := os.Chmod(dstPath, srcInfo.Mode().Perm())
		if chmodErr != nil {
			return chmodErr
		}

		entries, readDirErr := os.ReadDir(srcPath)
		if readDirErr != nil {
			return readDirErr
		}

		for _, entry := range entries {
			ctxErr = ctx.Err()
			if ctxErr != nil {
				return ctxErr
			}

			childSrcPath := filepath.Join(srcPath, entry.Name())
			childDstPath := filepath.Join(dstPath, entry.Name())

			childErr := p.copyPath(
				ctx,
				childSrcPath,
				childDstPath,
				overwrite,
			)
			if childErr != nil {
				return childErr
			}
		}

		return nil
	}

	if !srcInfo.Mode().IsRegular() {
		return storageerrs.New(
			string(p.name),
			storageerrs.ErrCopy,
			srcPath,
		)
	}

	mkdirErr := os.MkdirAll(filepath.Dir(dstPath), 0o750)
	if mkdirErr != nil {
		return mkdirErr
	}

	flags := os.O_CREATE | os.O_WRONLY

	if overwrite {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_EXCL
	}

	// #nosec G304 -- paths are produced from validated provider refs.
	srcFile, openErr := os.Open(srcPath)
	if openErr != nil {
		return openErr
	}

	// #nosec G304 -- paths are produced from validated provider refs.
	dstFile, openErr := os.OpenFile(
		dstPath,
		flags,
		srcInfo.Mode().Perm(),
	)
	if openErr != nil {
		closeErr := srcFile.Close()
		if closeErr != nil {
			return errors.Join(openErr, closeErr)
		}

		return openErr
	}

	_, copyErr := copyContext(ctx, dstFile, srcFile)

	dstCloseErr := dstFile.Close()
	srcCloseErr := srcFile.Close()

	if copyErr != nil {
		return errors.Join(copyErr, dstCloseErr, srcCloseErr)
	}

	if dstCloseErr != nil {
		return errors.Join(dstCloseErr, srcCloseErr)
	}

	if srcCloseErr != nil {
		return srcCloseErr
	}

	chmodErr := os.Chmod(dstPath, srcInfo.Mode().Perm())
	if chmodErr != nil {
		return chmodErr
	}

	return nil
}

func (p *Provider) Commit(
	ctx context.Context,
	prepared *storage.Prepared,
	opts storage.CommitOptions,
) (*storage.CommitResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if prepared == nil {
		return nil, storageerrs.New(string(p.name), storageerrs.ErrInvalidOptions, "prepared storage is nil")
	}
	if prepared.Ref.Provider != p.name {
		return nil, storageerrs.New(string(p.name), storageerrs.ErrProviderMismatch, string(prepared.Ref.Provider))
	}

	path, err := p.pathFor(prepared.Ref)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, storageerrs.Wrap(string(p.name), storageerrs.ErrNotFound, prepared.Ref.String(), err)
		}
		return nil, storageerrs.Wrap(string(p.name), storageerrs.ErrStat, prepared.Ref.String(), err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, storageerrs.New(string(p.name), storageerrs.ErrInvalidRef, prepared.Ref.String())
	}

	if opts.ExpectedGeneration != "" {
		current, statErr := p.Stat(ctx, prepared.Ref)
		if statErr != nil {
			return nil, statErr
		}
		if current.Generation != opts.ExpectedGeneration {
			return nil, storageerrs.New(string(p.name), storageerrs.ErrConflict, prepared.Ref.String())
		}
	}

	switch opts.Durability {
	case "", storage.DurabilityDefault, storage.DurabilityNone:
	case storage.DurabilityData:
		if info.Mode().IsRegular() {
			syncErr := syncFile(path)
			if syncErr != nil {
				return nil, storageerrs.Wrap(string(p.name), storageerrs.ErrPut, prepared.Ref.String(), syncErr)
			}
		}
	case storage.DurabilityFull:
		if info.Mode().IsRegular() {
			syncErr := syncFile(path)
			if syncErr != nil {
				return nil, storageerrs.Wrap(string(p.name), storageerrs.ErrPut, prepared.Ref.String(), syncErr)
			}
		}
		directorySyncErr := syncDirectory(filepath.Dir(path))
		if directorySyncErr != nil {
			return nil, storageerrs.Wrap(string(p.name), storageerrs.ErrPut, prepared.Ref.String(), directorySyncErr)
		}
	default:
		return nil, storageerrs.New(string(p.name), storageerrs.ErrInvalidOptions, string(opts.Durability))
	}

	objectInfo, err := p.Stat(ctx, prepared.Ref)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	delete(p.dirty, prepared.ID)
	p.mu.Unlock()
	p.emit(storage.Event{
		Type: storage.EventCommitted, Time: time.Now(), PreparedID: prepared.ID,
		NodeID: prepared.NodeID, Ref: refPtr(prepared.Ref), Generation: objectInfo.Generation,
	})
	return &storage.CommitResult{Object: *objectInfo}, nil
}

func (p *Provider) CommitAsync(
	ctx context.Context,
	prepared *storage.Prepared,
	opts storage.CommitOptions,
) (storage.CommitOperation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if prepared == nil {
		return nil, storageerrs.New(string(p.name), storageerrs.ErrInvalidOptions, "prepared storage is nil")
	}
	op := newOperation(ctx, storage.OperationCommit, &prepared.Ref, nil)
	op.setPreparedID(prepared.ID)
	go func() {
		op.start()
		result, err := p.Commit(op.context(), prepared, opts)
		op.setCommitResult(result, err)
		op.finish(err)
	}()
	return op, nil
}

func (p *Provider) MoveAsync(
	ctx context.Context,
	src storage.Ref,
	dst storage.Ref,
	opts storage.MoveOptions,
) (storage.OperationHandle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	op := newOperation(ctx, storage.OperationMove, &src, &dst)
	go func() {
		op.start()
		op.finish(p.Move(op.context(), src, dst, opts))
	}()
	return op, nil
}

func (p *Provider) DeleteAsync(
	ctx context.Context,
	ref storage.Ref,
) (storage.OperationHandle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	op := newOperation(ctx, storage.OperationDelete, &ref, nil)
	go func() {
		op.start()
		op.finish(p.Delete(op.context(), ref))
	}()
	return op, nil
}

func refPtr(ref storage.Ref) *storage.Ref {
	return &ref
}

func syncFile(path string) error {
	// #nosec G304 -- caller supplies a validated provider-owned path.
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return errors.Join(err, file.Close())
	}
	return file.Close()
}

func syncDirectory(path string) error {
	// #nosec G304 -- caller supplies a validated provider-owned path.
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	if err := dir.Sync(); err != nil {
		return errors.Join(err, dir.Close())
	}
	return dir.Close()
}
