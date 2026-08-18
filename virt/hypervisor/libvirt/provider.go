// Package libvirt provides a local libvirt-backed domain provider.
package libvirt

import (
	libvirtgo "libvirt.org/go/libvirt"

	"github.com/tethux/tethux/virt"
	"github.com/tethux/tethux/virt/domain"
	"github.com/tethux/tethux/virt/hypervisor/libvirt/errs"
)

type Provider struct {
	conn *libvirtgo.Connect
}

var _ domain.Provider = (*Provider)(nil)

func New(uri string) (*Provider, error) {
	conn, err := libvirtgo.NewConnect(uri)
	if err != nil {
		return nil, errs.Wrap(errs.ErrConnect, uri, err)
	}
	return &Provider{conn: conn}, nil
}

func (p *Provider) Info() virt.ProviderInfo {
	return virt.ProviderInfo{
		Name: "libvirt", DisplayName: "libvirt", Kind: virt.ProviderKindDomain,
		Capabilities: virt.Capabilities{Console: true, Pause: true},
	}
}

func (p *Provider) Close() error {
	if p == nil || p.conn == nil {
		return nil
	}
	_, err := p.conn.Close()
	return err
}
