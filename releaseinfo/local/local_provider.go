package local

import (
	"context"
	"embed"
	_ "embed"
	"github.com/openshift/hypershift/releaseinfo"
)

//go:embed stream/patched.json
//go:embed coreosMetadata.yaml
var content embed.FS

var _ releaseinfo.Provider = (*Provider)(nil)

type Provider struct {
}

func (l Provider) Lookup(ctx context.Context, image string, pullSecret []byte) (*releaseinfo.ReleaseImage, error) {
	byteValue, err := content.ReadFile("stream/patched.json")
	if err != nil {
		return nil, err
	}
	imageStream, err := releaseinfo.DeserializeImageStream(byteValue)
	if err != nil {
		return nil, err
	}

	meta, err := content.ReadFile("coreosMetadata.yaml")
	if err != nil {
		return nil, err
	}

	coreOSMeta, err := releaseinfo.DeserializeImageMetadata(meta)
	if err != nil {
		return nil, err
	}

	return &releaseinfo.ReleaseImage{
		ImageStream:    imageStream,
		StreamMetadata: coreOSMeta,
	}, nil
}
