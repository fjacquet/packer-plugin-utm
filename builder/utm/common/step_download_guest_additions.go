// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/multistep/commonsteps"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
)

// UTM version to guest additions version map
var additionsVersionMap = map[string]string{
	"4.6.4": "0.229.2",
}

type guestAdditionsUrlTemplate struct {
	Version string
}

// This step uploads a file containing the UTM version, which
// can be useful for various provisioning reasons.
//
// Produces:
//
//	guest_additions_path string - Path to the guest additions.
type StepDownloadGuestAdditions struct {
	GuestAdditionsMode       string
	GuestAdditionsURL        string
	GuestAdditionsSHA256     string
	GuestAdditionsTargetPath string
	Ctx                      interpolate.Context
}

func (s *StepDownloadGuestAdditions) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	driver := state.Get("driver").(Driver)
	ui := state.Get("ui").(packersdk.Ui)

	// If we've disabled guest additions, don't download
	if s.GuestAdditionsMode == GuestAdditionsModeDisable {
		log.Println("Not downloading guest additions since it is disabled.")
		return multistep.ActionContinue
	}

	// Get UTM version
	version, err := driver.Version()
	if err != nil {
		state.Put("error", fmt.Errorf("error reading version for guest additions download: %s", err))
		return multistep.ActionHalt
	}

	if newVersion, ok := additionsVersionMap[version]; ok {
		log.Printf("Rewriting guest additions version: %s to %s", version, newVersion)
		version = newVersion
	}

	additionsName := fmt.Sprintf("utm-guest-tools-%s.iso", "latest")

	// Use provided version or get it from getutm.app
	var checksum string

	checksumType := "sha256"

	// Initialize the template context so we can interpolate some variables..
	s.Ctx.Data = &guestAdditionsUrlTemplate{
		Version: version,
	}

	// Interpolate any user-variables specified within the guest_additions_url
	url, err := interpolate.Render(s.GuestAdditionsURL, &s.Ctx)
	if err != nil {
		err := fmt.Errorf("error preparing guest additions url: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	// If this resulted in an empty url, then ask the driver about it.
	if url == "" {
		log.Printf("guest_additions_url is blank; querying driver for iso.")
		url, err = driver.GuestToolsIsoPath()

		if err == nil {
			checksumType = "none"
		} else {
			ui.Error(err.Error())
			url = fmt.Sprintf(
				"https://getutm.app/downloads/%s", additionsName)
		}
	}

	// The driver couldn't even figure it out, so fail hard.
	if url == "" {
		err := fmt.Errorf("couldn't detect guest additions URL.\n" +
			"Please specify `guest_additions_url` manually")
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	// Figure out a default checksum here
	if checksumType != "none" {
		if s.GuestAdditionsSHA256 != "" {
			checksum = s.GuestAdditionsSHA256
		} else {
			// Skip checksum verification for default guest additions ISO
			// since UTM doesn't provide checksums and versions change frequently
			log.Println("Skipping checksum verification for default guest additions ISO")
			checksumType = "none"
		}
	}

	log.Printf("Guest additions URL: %s", url)

	// Build checksum string with type prefix
	checksumWithType := checksum
	if checksumType != "none" && checksum != "" {
		checksumWithType = fmt.Sprintf("%s:%s", checksumType, checksum)
	}

	// We're good, so let's go ahead and download this thing..
	downStep := &commonsteps.StepDownload{
		Checksum:    checksumWithType,
		Description: "Guest additions",
		ResultKey:   "guest_additions_path",
		TargetPath:  s.GuestAdditionsTargetPath,
		Url:         []string{url},
		Extension:   "iso",
	}

	return downStep.Run(ctx, state)
}

func (s *StepDownloadGuestAdditions) Cleanup(state multistep.StateBag) {}
