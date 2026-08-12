package local

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"strings"

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
}

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
		name: DefaultName,
		root: abs,
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
			return nil, storageerrs.Wrap(string(p.name), storageerrs.ErrNotFound, ref.String(), err)
		}
		return nil, storageerrs.Wrap(string(p.name), storageerrs.ErrStat, ref.String(), err)
	}
	var checksum *storage.Checksum

	if info.Mode().IsRegular() {
		checksum, err = p.checksum(ctx, path)
		if err != nil {
			return nil, storageerrs.Wrap(
				string(p.name),
				storageerrs.ErrChecksum,
				ref.String(),
				err,
			)
		}
	}

	generation := storage.Generation("")
	if checksum != nil {
		generation = storage.Generation(checksum.Value)
	}

	return &storage.ObjectInfo{
		Ref:         ref,
		Type:        objectType(info),
		Size:        info.Size(),
		ModTime:     info.ModTime(),
		ContentType: mime.TypeByExtension(filepath.Ext(path)),
		Checksum:    checksum,
		Generation:  generation,
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
) ([]storage.ObjectInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	path, err := p.pathFor(prefix)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, storageerrs.Wrap(string(p.name), storageerrs.ErrList, prefix.String(), err)
	}

	out := make([]storage.ObjectInfo, 0, len(entries))

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		info, err := entry.Info()
		if err != nil {
			return nil, storageerrs.Wrap(string(p.name), storageerrs.ErrList, entry.Name(), err)
		}

		key := filepath.ToSlash(
			filepath.Join(string(prefix.Key), entry.Name()),
		)

		out = append(out, storage.ObjectInfo{
			Ref: storage.Ref{
				Provider: p.name,
				Key:      storage.Key(key),
			},
			Type:    objectType(info),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}

	return out, nil
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
		return nil, storageerrs.New(string(p.name), storageerrs.ErrUnsupportedMode, string(mode))
	}

	path, err := p.pathFor(req.Ref)
	if err != nil {
		return nil, err
	}

	created := false
	info, statErr := os.Stat(path)
	if statErr != nil {
		if !errors.Is(statErr, fs.ErrNotExist) {
			return nil, storageerrs.Wrap(string(p.name), storageerrs.ErrStat, req.Ref.String(), statErr)
		}
		if !req.Create {
			return nil, storageerrs.Wrap(string(p.name), storageerrs.ErrNotFound, req.Ref.String(), statErr)
		}

		switch req.ResourceType {
		case storage.ResourceTypeDirectory:
			mkdirErr := os.MkdirAll(path, 0o750)
			if mkdirErr != nil {
				return nil, storageerrs.Wrap(string(p.name), storageerrs.ErrCreate, req.Ref.String(), mkdirErr)
			}
			created = true

		case storage.ResourceTypeFile:
			mkdirErr := os.MkdirAll(filepath.Dir(path), 0o750)
			if mkdirErr != nil {
				return nil, storageerrs.Wrap(string(p.name), storageerrs.ErrCreate, req.Ref.String(), mkdirErr)
			}
			// #nosec G304 -- pathFor rejects absolute, escaping, and symlinked refs.
			file, openErr := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if openErr != nil {
				return nil, storageerrs.Wrap(string(p.name), storageerrs.ErrCreate, req.Ref.String(), openErr)
			}
			if closeErr := file.Close(); closeErr != nil {
				return nil, storageerrs.Wrap(string(p.name), storageerrs.ErrCreate, req.Ref.String(), closeErr)
			}
			created = true

		default:
			return nil, storageerrs.New(string(p.name), storageerrs.ErrInvalidResourceType, req.Ref.String())
		}

		info, err = os.Stat(path)
		if err != nil {
			return nil, storageerrs.Wrap(string(p.name), storageerrs.ErrStat, req.Ref.String(), err)
		}
	}

	if err := validateResourceType(req.ResourceType, info); err != nil {
		return nil, storageerrs.Wrap(string(p.name), storageerrs.ErrInvalidResourceType, req.Ref.String(), err)
	}

	ownership := storage.OwnershipExternal
	if created {
		ownership = storage.OwnershipNode
	}

	return &storage.Prepared{
		ID:         storage.PreparedID(uuid.NewString()),
		Ref:        req.Ref,
		NodeID:     req.NodeID,
		AccessMode: req.AccessMode,
		Ownership:  ownership,
		Location: storage.RuntimeLocation{
			Kind:  storage.LocationPath,
			Value: path,
		},
	}, nil
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
		return storageerrs.New(string(p.name), storageerrs.ErrInvalidOptions, "prepared storage is nil")
	}

	return nil
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

func (p *Provider) checksum(ctx context.Context, path string) (*storage.Checksum, error) {
	ctxErr := ctx.Err()
	if ctxErr != nil {
		return nil, ctxErr
	}

	// #nosec G304 -- paths are produced from a validated storage ref.
	file, openErr := os.Open(path)
	if openErr != nil {
		return nil, openErr
	}
	defer file.Close()

	hash := sha256.New()

	if _, hashErr := copyContext(ctx, hash, file); hashErr != nil {
		return nil, hashErr
	}
	return &storage.Checksum{
		Algorithm: storage.ChecksumSHA256,
		Value:     hex.EncodeToString(hash.Sum(nil)),
	}, nil
}
