package docker

import "context"

func (d *Driver) SetAutoStop(context.Context, string, int) error {
	return nil
}

func (d *Driver) SetAutoArchive(context.Context, string, int) error {
	return nil
}
