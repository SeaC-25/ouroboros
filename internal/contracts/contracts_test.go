package contracts

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/domain"
	"github.com/ouroboros/ouroboros/internal/toolchain"
)

func TestBuiltInCapabilityContractsAreAligned(t *testing.T) {
	if err := ValidateBuiltIn(filepath.Join("..", "..")); err != nil {
		t.Fatalf("built-in contract validation failed: %v", err)
	}
}

func TestValidateRejectsCatalogCapabilityOutsideDeclaredPack(t *testing.T) {
	manifests, err := toolchain.LoadManifests(filepath.Join("..", "..", "toolchains"))
	if err != nil {
		t.Fatalf("load manifests: %v", err)
	}
	catalog := toolchain.BuiltInAppSecTools()
	for i := range catalog {
		if catalog[i].Name == "nmap" {
			catalog[i].Capabilities = []string{"web.discovery"}
			break
		}
	}

	err = Validate(Inputs{
		Manifests:   manifests,
		ToolCatalog: catalog,
		DomainPacks: BuiltInDomainPacks(),
		Patterns:    BuiltInPatternContracts(),
	})
	if err == nil {
		t.Fatalf("expected catalog capability outside pack manifest to fail")
	}
	if !strings.Contains(err.Error(), "tool catalog entry nmap capability web.discovery is not declared by manifest infra-pack") {
		t.Fatalf("expected pack-specific capability error, got %v", err)
	}
}

func TestValidateRejectsCatalogDependencyOutsideDeclaredPack(t *testing.T) {
	manifests, err := toolchain.LoadManifests(filepath.Join("..", "..", "toolchains"))
	if err != nil {
		t.Fatalf("load manifests: %v", err)
	}
	catalog := toolchain.BuiltInAppSecTools()
	for i := range catalog {
		if catalog[i].Name == "nmap" {
			catalog[i].Dependency = "missing-nmap"
			break
		}
	}

	err = Validate(Inputs{
		Manifests:   manifests,
		ToolCatalog: catalog,
		DomainPacks: BuiltInDomainPacks(),
		Patterns:    BuiltInPatternContracts(),
	})
	if err == nil {
		t.Fatalf("expected catalog dependency outside pack manifest to fail")
	}
	if !strings.Contains(err.Error(), "tool catalog entry nmap dependency missing-nmap is not declared by manifest infra-pack") {
		t.Fatalf("expected pack-specific dependency error, got %v", err)
	}
}

func TestValidateRejectsManifestParserOutsideRegistry(t *testing.T) {
	manifests, err := toolchain.LoadManifests(filepath.Join("..", "..", "toolchains"))
	if err != nil {
		t.Fatalf("load manifests: %v", err)
	}
	for i := range manifests {
		if manifests[i].Name == "web-api-pack" {
			manifests[i].Parser = "drifted_parser"
			break
		}
	}

	err = Validate(Inputs{
		Manifests:   manifests,
		ToolCatalog: toolchain.BuiltInAppSecTools(),
		DomainPacks: BuiltInDomainPacks(),
		Patterns:    BuiltInPatternContracts(),
	})
	if err == nil {
		t.Fatalf("expected manifest parser outside registry to fail")
	}
	if !strings.Contains(err.Error(), "manifest web-api-pack parser drifted_parser is not registered") {
		t.Fatalf("expected manifest parser registry error, got %v", err)
	}
}

func TestValidateRejectsCatalogParserOutsideRegistry(t *testing.T) {
	manifests, err := toolchain.LoadManifests(filepath.Join("..", "..", "toolchains"))
	if err != nil {
		t.Fatalf("load manifests: %v", err)
	}
	catalog := toolchain.BuiltInAppSecTools()
	for i := range catalog {
		if catalog[i].Name == "httpx" {
			catalog[i].Parser = "drifted_parser"
			break
		}
	}

	err = Validate(Inputs{
		Manifests:   manifests,
		ToolCatalog: catalog,
		DomainPacks: BuiltInDomainPacks(),
		Patterns:    BuiltInPatternContracts(),
	})
	if err == nil {
		t.Fatalf("expected catalog parser outside registry to fail")
	}
	if !strings.Contains(err.Error(), "tool catalog entry httpx parser drifted_parser is not registered") {
		t.Fatalf("expected catalog parser registry error, got %v", err)
	}
}

func TestValidateRejectsCatalogParserUnsupportedByOperation(t *testing.T) {
	manifests, err := toolchain.LoadManifests(filepath.Join("..", "..", "toolchains"))
	if err != nil {
		t.Fatalf("load manifests: %v", err)
	}
	catalog := toolchain.BuiltInAppSecTools()
	for i := range catalog {
		if catalog[i].Name == "nuclei" {
			catalog[i].Parser = "httpx_jsonl"
			break
		}
	}

	err = Validate(Inputs{
		Manifests:   manifests,
		ToolCatalog: catalog,
		DomainPacks: BuiltInDomainPacks(),
		Patterns:    BuiltInPatternContracts(),
	})
	if err == nil {
		t.Fatalf("expected catalog parser unsupported by operation to fail")
	}
	if !strings.Contains(err.Error(), "tool catalog entry nuclei parser httpx_jsonl is not supported by operation managed_binary/template_scan") {
		t.Fatalf("expected operation parser support error, got %v", err)
	}
}

func TestValidateRejectsNilDomainPackWithoutPanic(t *testing.T) {
	manifests, err := toolchain.LoadManifests(filepath.Join("..", "..", "toolchains"))
	if err != nil {
		t.Fatalf("load manifests: %v", err)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("validate must return an error instead of panicking: %v", recovered)
		}
	}()

	err = Validate(Inputs{
		Manifests:   manifests,
		ToolCatalog: toolchain.BuiltInAppSecTools(),
		DomainPacks: []domain.Pack{nil},
		Patterns:    BuiltInPatternContracts(),
	})
	if err == nil {
		t.Fatalf("expected nil domain pack to fail")
	}
	if !strings.Contains(err.Error(), "validate domain pack 0: domain pack is required") {
		t.Fatalf("expected nil domain pack error, got %v", err)
	}
}

func TestValidateRejectsBlankCatalogFields(t *testing.T) {
	manifests, err := toolchain.LoadManifests(filepath.Join("..", "..", "toolchains"))
	if err != nil {
		t.Fatalf("load manifests: %v", err)
	}
	catalog := append([]toolchain.BuiltInTool{
		toolchain.BuiltInTool{Name: "", Pack: "web-api-pack", Capabilities: []string{"web.discovery"}},
		toolchain.BuiltInTool{Name: "missing-pack", Pack: "", Capabilities: []string{"web.discovery"}},
		toolchain.BuiltInTool{Name: "blank-capability", Pack: "web-api-pack", Capabilities: []string{" "}},
	}, toolchain.BuiltInAppSecTools()...)

	err = Validate(Inputs{
		Manifests:   manifests,
		ToolCatalog: catalog,
		DomainPacks: BuiltInDomainPacks(),
		Patterns:    BuiltInPatternContracts(),
	})
	if err == nil {
		t.Fatalf("expected blank catalog fields to fail")
	}
	for _, want := range []string{
		"tool catalog entry 0 name is required",
		"tool catalog entry missing-pack pack is required",
		"tool catalog entry blank-capability capability 0 is required",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected %q in errors, got %v", want, err)
		}
	}
}
