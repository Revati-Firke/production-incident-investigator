package tools

import (
	domaintool "github.com/Revati-Firke/production-incident-investigator/internal/domain/tool"
)

// Authorize checks whether a tool may be executed with the given approval flag.
func Authorize(level PermissionLevel, approved bool) error {
	switch level {
	case PermissionReadOnly, PermissionAutonomous:
		return nil
	case PermissionRequiresApproval:
		if !approved {
			return domaintool.ErrApprovalRequired
		}
		return nil
	default:
		return domaintool.ErrPermissionDenied
	}
}

// ToDomainPermission converts agent permission to domain permission.
func ToDomainPermission(level PermissionLevel) domaintool.PermissionLevel {
	return domaintool.PermissionLevel(level)
}
