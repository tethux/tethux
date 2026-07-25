package ci

import (
	"path/filepath"
	"runtime"
	"time"
)

func BuiltinWorkflows(root string) []Workflow {
	artifactDir := filepath.Join(root, "results", "current", "artifacts")
	goTest := Step{
		Name: "go-test", Command: "go", Args: []string{"test", "./...", "-json"}, Dir: root,
		Outputs:       []Output{{Name: "go-test", Path: filepath.Join(artifactDir, "go-test.jsonl"), Kind: "application/x-ndjson"}},
		CaptureStdout: filepath.Join(artifactDir, "go-test.jsonl"),
		Timeout:       30 * time.Minute,
	}
	normal := Workflow{
		Name: "normal", Description: "format, lint, unit, build, and Nix checks",
		Archive: ArchiveMetadata{Workflow: "normal"},
		Steps: []Step{
			{Name: "lint", Command: "golangci-lint", Args: []string{"run", "-c", ".golangci.yml"}, Dir: root, Timeout: 20 * time.Minute},
			goTest,
			{Name: "build", Command: "go", Args: []string{"build", "./cmd/tethux"}, Dir: root, DependsOn: []string{"go-test"}},
			{Name: "nix-host-100", Command: "nix", Args: []string{"eval", ".#nixosConfigurations.canary-10-0-0-100.config.system.build.toplevel.drvPath", "--extra-experimental-features", "nix-command flakes"}, Dir: root},
			{Name: "nix-host-78", Command: "nix", Args: []string{"eval", ".#nixosConfigurations.canary-former-10-0-0-12.config.system.build.toplevel.drvPath", "--extra-experimental-features", "nix-command flakes"}, Dir: root},
			{Name: "nix-proxmox", Command: "nix", Args: []string{"eval", ".#nixosConfigurations.canary-proxmox-vm-9901.config.system.build.toplevel.drvPath", "--extra-experimental-features", "nix-command flakes"}, Dir: root},
			{Name: "nix-checks", Command: "nix", Args: []string{"build", ".#checks." + nixArchitecture() + ".unit", ".#checks." + nixArchitecture() + ".build", "--extra-experimental-features", "nix-command flakes"}, Dir: root},
		},
	}
	return []Workflow{
		normal,
		{Name: "provider", Description: "container provider lifecycle", Steps: []Step{{
			Name: "provider", Command: "go",
			Args: []string{"run", "./cmd/tethux", "virt", "test", "--provider", "all", "--output", "json"},
			Dir:  root, Privilege: PrivilegeRoot, Timeout: 45 * time.Minute,
			Outputs:       []Output{{Name: "providers", Path: filepath.Join(artifactDir, "provider-results.jsonl"), Kind: "application/x-ndjson"}},
			CaptureStdout: filepath.Join(artifactDir, "provider-results.jsonl"),
		}}},
		{Name: "topology", Description: "container topology", Steps: []Step{{
			Name: "topology", Command: "go", Args: []string{"run", "./tools/bridge/example/container-udp", "--runtime", "all"},
			Dir: root, Privilege: PrivilegeRoot, Timeout: 45 * time.Minute,
		}}},
		{Name: "bridge-backends", Description: "exact-frame backend conformance", Steps: []Step{{
			Name: "bridge-backends", Command: "go", Args: []string{"run", "./tools/bridge/testing/backend-smoke"},
			Dir: root, Privilege: PrivilegeRoot, Timeout: 30 * time.Minute,
			Outputs:       []Output{{Name: "bridge-backends", Path: filepath.Join(artifactDir, "bridge-backends.jsonl"), Kind: "application/x-ndjson"}},
			CaptureStdout: filepath.Join(artifactDir, "bridge-backends.jsonl"),
		}}},
		{Name: "hypervisors", Description: "network primitives and available hypervisor checks", Steps: []Step{
			{Name: "dummy-add", Command: "ip", Args: []string{"link", "add", "tethux-dummy0", "type", "dummy"}, Privilege: PrivilegeRoot},
			{Name: "dummy-address", Command: "ip", Args: []string{"addr", "add", "198.51.100.10/32", "dev", "tethux-dummy0"}, Privilege: PrivilegeRoot, DependsOn: []string{"dummy-add"}},
			{Name: "dummy-delete", Command: "ip", Args: []string{"link", "delete", "tethux-dummy0"}, Privilege: PrivilegeRoot, DependsOn: []string{"dummy-address"}, Always: true},
			{Name: "tap-add", Command: "ip", Args: []string{"tuntap", "add", "dev", "tethux-tap0", "mode", "tap"}, Privilege: PrivilegeRoot},
			{Name: "tap-delete", Command: "ip", Args: []string{"link", "delete", "tethux-tap0"}, Privilege: PrivilegeRoot, DependsOn: []string{"tap-add"}, Always: true},
			{Name: "qemu-version", Command: "qemu-system-x86_64", Args: []string{"--version"}, Timeout: time.Minute, AllowMissing: true},
			{Name: "virsh-list", Command: "virsh", Args: []string{"list", "--all"}, Timeout: time.Minute, AllowMissing: true},
			{Name: "dynamips-version", Command: "dynamips", Args: []string{"--version"}, Timeout: time.Minute, AllowMissing: true},
			{Name: "virtualbox-list", Command: "VBoxManage", Args: []string{"list", "vms"}, Timeout: time.Minute, AllowMissing: true},
			{Name: "vmware-version", Command: "vmrun", Timeout: time.Minute, AllowMissing: true},
		}, ContinueOnError: true},
	}
}

func nixArchitecture() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64-linux"
	case "arm64":
		return "aarch64-linux"
	default:
		return runtime.GOARCH + "-linux"
	}
}
