// Copyright (c) 2022 Gitpod GmbH. All rights reserved.
// Licensed under the GNU Affero General Public License (AGPL).
// See License-AGPL.txt in the project root for license information.

package cgroup

import (
	"context"
	"fmt"
	"time"

	"github.com/gitpod-io/gitpod/common-go/log"
	"github.com/intel/goresctrl/pkg/blockio"
)

type IOLimiterV1 struct {
}

func NewIOLimiterV1(writeBytesPerSecond, readBytesPerSecond, writeIOPs, readIOPs int64) *IOLimiterV1 {
	err := blockio.SetConfig(&blockio.Config{
		Classes: map[string][]blockio.DevicesParameters{
			"default": {
				{
					Devices:           []string{"/dev/sd[a-z]", "/dev/md[0-99]"},
					ThrottleReadBps:   fmt.Sprintf("%v", readBytesPerSecond),
					ThrottleWriteBps:  fmt.Sprintf("%v", writeBytesPerSecond),
					ThrottleReadIOPS:  fmt.Sprintf("%v", readIOPs),
					ThrottleWriteIOPS: fmt.Sprintf("%v", writeIOPs),
				},
			},
			"reset": {
				{
					Devices:           []string{"/dev/sd[a-z]", "/dev/md[0-99]"},
					ThrottleReadBps:   fmt.Sprintf("%v", 0),
					ThrottleWriteBps:  fmt.Sprintf("%v", 0),
					ThrottleReadIOPS:  fmt.Sprintf("%v", 0),
					ThrottleWriteIOPS: fmt.Sprintf("%v", 0),
				},
			},
		},
	}, true)
	if err != nil {
		log.WithError(err).Fatal("cannot start daemon")
	}

	return &IOLimiterV1{}
}

func (c *IOLimiterV1) Name() string  { return "iolimiter-v1" }
func (c *IOLimiterV1) Type() Version { return Version1 }

func (c *IOLimiterV1) Apply(ctx context.Context, basePath, cgroupPath string) error {
	go func() {
		log.WithField("cgroupPath", cgroupPath).Debug("starting io limiting")
		// We are racing workspacekit and the interaction with disks.
		// If we did this just once there's a chance we haven't interacted with all
		// devices yet, and hence would not impose IO limits on them.
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				// Prior to shutting down though, we need to reset the IO limits to ensure we don't have
				// processes stuck in the uninterruptable "D" (disk sleep) state. This would prevent the
				// workspace pod from shutting down.

				err := blockio.SetCgroupClass(cgroupPath, "reset")
				if err != nil {
					log.WithError(err).WithField("cgroupPath", cgroupPath).Error("cannot write IO limits")
				}
				log.WithField("cgroupPath", cgroupPath).Debug("stopping io limiting")
				return
			case <-ticker.C:
				err := blockio.SetCgroupClass(cgroupPath, "default")
				if err != nil {
					log.WithError(err).WithField("cgroupPath", cgroupPath).Error("cannot write IO limits")
				}
			}
		}
	}()
	return nil
}
