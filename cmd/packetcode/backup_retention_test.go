package main

import (
	"testing"
	"time"

	"github.com/packetcode/packetcode/internal/config"
	"github.com/packetcode/packetcode/internal/session"
)

func TestBackupRetention(t *testing.T) {
	tests := []struct {
		name     string
		behavior config.BehaviorConfig
		want     time.Duration
	}{
		{"unset uses the built-in default", config.BehaviorConfig{}, session.DefaultBackupMaxAge},
		{"a configured window is honoured", config.BehaviorConfig{BackupRetentionDays: 3}, 72 * time.Hour},
		{"a negative count falls back like every other cap", config.BehaviorConfig{BackupRetentionDays: -5}, session.DefaultBackupMaxAge},
		{"the disable switch beats a configured window", config.BehaviorConfig{BackupRetentionDays: 3, BackupPruneDisabled: true}, 0},
		{"the disable switch beats an unset window", config.BehaviorConfig{BackupPruneDisabled: true}, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := backupRetention(tc.behavior); got != tc.want {
				t.Errorf("backupRetention() = %s, want %s", got, tc.want)
			}
		})
	}
}
