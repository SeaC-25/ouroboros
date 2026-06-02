package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func requireRuntimeWorkerSideEffectSafetyAuthorization(root string, now time.Time) error {
	config, err := LoadScopeConfig(filepath.Join(root, "scope.yaml"))
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("load scope config for worker runtime safety gate: %w", err)
		}
		config = defaultRuntimeWorkerSafetyScopeConfig()
	}
	if err := ValidateScopeConfig(config); err != nil {
		return fmt.Errorf("invalid scope config for worker runtime safety gate: %w", err)
	}
	authorization, authorizationFound, err := loadWorkspaceAuthorization(filepath.Join(root, "authorization.yaml"))
	if err != nil {
		return fmt.Errorf("invalid authorization.yaml for worker runtime safety gate: %w", err)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	strictErrors := scopeConfigSafetyAuthorizationErrors(config, authorization, authorizationFound, now)
	if len(strictErrors) == 0 {
		return nil
	}
	return fmt.Errorf("worker runtime side effect requires current safety bypass authorization: %s", strings.Join(strictErrors, "; "))
}

func defaultRuntimeWorkerSafetyScopeConfig() ScopeConfig {
	config := ScopeConfig{Version: 1}
	config.Scope.TargetPolicy = "enforce"
	config.Execution.Isolation = "process"
	return config
}
