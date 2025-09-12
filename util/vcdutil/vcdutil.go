package vcdutil

import (
	"github.com/vmware/go-vcloud-director/v2/govcd"
)

func IgnoreNotFound(err error) error {
	if govcd.ContainsNotFound(err) {
		return nil
	}

	return err
}
