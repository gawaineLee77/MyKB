package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/enterprise"
)

func handleInstallCommand(ctx context.Context, s *enterprise.Service, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("install.command_required")
	}
	flags := flag.NewFlagSet("install", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	adopt := flags.String("admin-id", "", "explicitly recover an ambiguous account creation")
	tenantID := flags.Uint64("default-space-id", 0, "explicitly recover an ambiguous space creation")
	keyID := flags.Uint64("member-key-id", 0, "explicitly recover member credential creation")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
		return fmt.Errorf("install.arguments_invalid")
	}
	input := enterprise.InstallInput{Email: os.Getenv("MINDCREEK_INSTALL_ADMIN_EMAIL"), Username: os.Getenv("MINDCREEK_INSTALL_ADMIN_USERNAME"), PasswordFile: os.Getenv("MINDCREEK_INSTALL_ADMIN_PASSWORD_FILE"), AdoptAdminID: *adopt}
	var result enterprise.Installation
	var err error
	switch args[0] {
	case "prepare":
		result, err = s.Prepare(ctx, input)
	case "confirm-admin":
		result, err = s.ConfirmAdmin(ctx, input)
	case "status":
		result, err = s.Store.Installation(ctx)
		if errors.Is(err, enterprise.ErrNotFound) {
			result = enterprise.Installation{Stage: "new"}
			err = nil
		}
	case "repair-member-key":
		result, err = s.RepairMemberKey(ctx, os.Getenv("MINDCREEK_RECOVERY_OWNER_BEARER_FILE"), os.Getenv("MINDCREEK_MEMBER_KEY_FILE"), *keyID)
	case "default-space":
		result, err = s.DefaultWorkspace(ctx, enterprise.WorkspaceInput{PasswordFile: input.PasswordFile, Name: os.Getenv("MINDCREEK_DEFAULT_SPACE_NAME"), Description: os.Getenv("MINDCREEK_DEFAULT_SPACE_DESCRIPTION"), MemberKeyFile: os.Getenv("MINDCREEK_MEMBER_KEY_FILE"), AdoptTenantID: *tenantID, AdoptMemberKeyID: *keyID})
	default:
		return fmt.Errorf("install.command_invalid")
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
