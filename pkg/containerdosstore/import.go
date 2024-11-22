/*
Copyright © 2024 SUSE LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package containerdosstore

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/containerd/containerd/v2/client"
)

func (c *ContainerdOSStore) Import(reader io.Reader, opts ...client.ImportOpt) ([]client.Image, error) {
	if !c.IsInitiated() {
		return nil, errors.New(missInitErrMsg)
	}

	// TODO add unpack option
	// TODO handle lease

	images := []client.Image{}
	imgs, err := c.cli.Import(c.ctx, reader, opts...)
	if err != nil {
		return nil, err
	}
	for _, img := range imgs {
		images = append(images, client.NewImage(c.cli, img))
	}

	return images, nil
}

func (c *ContainerdOSStore) ImportFile(file string, opts ...client.ImportOpt) (img []client.Image, retErr error) {
	r, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer func() {
		err := r.Close()
		if err != nil && retErr == nil {
			retErr = err
		}
	}()

	img, err = c.Import(r, opts...)
	if err != nil {
		return nil, err
	}

	return img, nil
}

func (c *ContainerdOSStore) SingleImportFile(file string, opts ...client.ImportOpt) (client.Image, error) {
	images, err := c.ImportFile(file, opts...)
	if err != nil {
		return nil, err
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("something went wrong, no images imported")
	}

	if len(images) > 1 {
		var dErrs []error
		delImg := func(img client.Image) {
			err = c.Delete(img.Name())
			if err != nil {
				c.log.Errorf("cound not delete imported image '%s': %v", img.Name(), err)
				dErrs = append(dErrs, err)
			}
		}

		c.log.Warnf("imported '%d' images. Only keeping first one", len(images))
		for _, img := range images[1:] {
			delImg(img)
		}
		if len(dErrs) > 0 {
			delImg(images[0])
			return nil, fmt.Errorf("failed removing imported images")
		}
	}
	return images[0], nil
}
