package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/tethux/tethux/internal/libtethux/storage"
)

const DefaultName storage.ProviderName = "local"

type Provider struct {
	name storage.ProviderName
	root string
}

type Option func(*Provider)

func WithName(name storage.ProviderName) Option {
	return func(p *Provider) {
		p.name = name
	}
}

func New(root string, opts ...Option) (*Provider, error) {
	if root == "" {
		return nil, errors.New("local storage root is empty")
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve local storage root: %w", err)
	}

	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("create local storage root %q: %w", abs, err)
	}

	p := &Provider{
		name: DefaultName,
		root: abs,
	}

	for _, opt := range opts {
		opt(p)
	}

	if p.name == "" {
		return nil, errors.New("local storage provider name is empty")
	}

	return p, nil
}

func (p *Provider) Name() storage.ProviderName {
	return p.name
}

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
		return nil, fmt.Errorf("stat %s: %w", ref, err)
	}

	return &storage.ObjectInfo{
		Ref:     ref,
		Type:    objectType(info),
		Size:    info.Size(),
		ModTime: info.ModTime(),
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

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", ref, err)
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
		return errors.New("storage input reader is nil")
	}

	path, err := p.pathFor(ref)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create parent directory for %s: %w", ref, err)
	}

	mode := fs.FileMode(opts.Mode)
	if mode == 0 {
		mode = 0o644
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".tethux-*")
	if err != nil {
		return fmt.Errorf("create temporary file for %s: %w", ref, err)
	}

	tmpPath := tmp.Name()

	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}

	if err := tmp.Chmod(mode); err != nil {
		cleanup()
		return fmt.Errorf("chmod temporary file for %s: %w", ref, err)
	}

	if _, err := copyContext(ctx, tmp, r); err != nil {
		cleanup()
		return fmt.Errorf("write %s: %w", ref, err)
	}

	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync %s: %w", ref, err)
	}

	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temporary file for %s: %w", ref, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace %s: %w", ref, err)
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
		return fmt.Errorf("delete %s: %w", ref, err)
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
		return nil, fmt.Errorf("list %s: %w", prefix, err)
	}

	out := make([]storage.ObjectInfo, 0, len(entries))

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("inspect %q: %w", entry.Name(), err)
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
	req storage.PrepareRequest,
) (*storage.Prepared, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	path, err := p.pathFor(req.Ref)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("prepare %s: %w", req.Ref, err)
	}

	return &storage.Prepared{
		Ref: req.Ref,
		Location: storage.RuntimeLocation{
			Kind:  storage.LocationPath,
			Value: path,
		},
	}, nil
}

func (p *Provider) Release(
	ctx context.Context,
	prepared *storage.Prepared,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if prepared == nil {
		return errors.New("prepared storage is nil")
	}

	return nil
}

func (p *Provider) pathFor(ref storage.Ref) (string, error) {
	if err := ref.Validate(); err != nil {
		return "", err
	}

	if ref.Provider != p.name {
		return "", fmt.Errorf(
			"storage ref provider %q does not match %q",
			ref.Provider,
			p.name,
		)
	}

	key := filepath.FromSlash(string(ref.Key))

	if filepath.IsAbs(key) {
		return "", fmt.Errorf("storage key must be relative: %q", ref.Key)
	}

	clean := filepath.Clean(key)

	if clean == "." || clean == "" {
		return "", fmt.Errorf("invalid storage key %q", ref.Key)
	}

	if clean == ".." ||
		strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("storage key escapes root: %q", ref.Key)
	}

	full := filepath.Join(p.root, clean)

	rel, err := filepath.Rel(p.root, full)
	if err != nil {
		return "", fmt.Errorf("validate storage path: %w", err)
	}

	if rel == ".." ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("storage key escapes root: %q", ref.Key)
	}
	current := p.root
	for _, component := range strings.Split(clean, string(filepath.Separator)) {
		current = filepath.Join(current, component)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, fs.ErrNotExist) {
			break
		}
		if statErr != nil {
			return "", fmt.Errorf("inspect storage path %q: %w", current, statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("storage key contains symlink component: %q", ref.Key)
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
