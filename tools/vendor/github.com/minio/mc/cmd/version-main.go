// Copyright (c) 2015-2021 MinIO, Inc.
//
// This file is part of MinIO Object Storage stack
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package cmd

import "github.com/minio/cli"

var versionSubcommands = []cli.Command{
	versionEnableCmd,
	versionSuspendCmd,
	versionInfoCmd,
}

var versionCmd = cli.Command{
	Name:            "version",
	Usage:           "manage bucket versioning",
	HideHelpCommand: true,
	Action:          mainVersion,
	Before:          setGlobalsFromContext,
	Flags:           globalFlags,
	Subcommands:     versionSubcommands,
}

// mainVersion is the handle for "mc version" command.
func mainVersion(ctx *cli.Context) error {
	commandNotFound(ctx, versionSubcommands)
	return nil
	// Sub-commands like "info", "enable", "suspend" have their own main.
}
