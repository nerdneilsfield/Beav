package cleaners_test

import (
	"strings"
	"testing"

	"github.com/dengqi/beav/cleaners"
	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/cleaner/registry"
)

// TestEmbeddedRegistryValidates verifies that all embedded builtin cleaner definitions pass validation.
// TestEmbeddedRegistryValidates 验证所有嵌入的内置清理器定义通过验证。
func TestEmbeddedRegistryValidates(t *testing.T) {
	loaded, err := registry.LoadBuiltin(cleaners.Builtin)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) == 0 {
		t.Fatal("no embedded cleaners")
	}
	for _, l := range loaded {
		if err := registry.Validate(l.Cleaner); err != nil {
			t.Errorf("%s: %v", l.From, err)
		}
	}
}

// TestEmbeddedRegistryContainsClosedProfiles verifies that every Task 2–4
// profile has one enabled builtin entry with an explicit age and impact.
func TestEmbeddedRegistryContainsClosedProfiles(t *testing.T) {
	want := map[string]struct {
		typ    model.ExecutorType
		impact model.Impact
	}{
		"pkg-homebrew":                  {model.TypeToolPrune, model.ImpactSafe},
		"lang-uv":                       {model.TypeToolPrune, model.ImpactSafe},
		"lang-pnpm":                     {model.TypeToolPrune, model.ImpactSafe},
		"dev-pre-commit":                {model.TypeToolPrune, model.ImpactSafe},
		"lang-conda":                    {model.TypeToolPrune, model.ImpactSafe},
		"test-cypress":                  {model.TypeToolPrune, model.ImpactSafe},
		"build-ccache":                  {model.TypeToolPrune, model.ImpactSafe},
		"ai-huggingface-prune":          {model.TypeToolPrune, model.ImpactHeavy},
		"dev-mise":                      {model.TypeToolPrune, model.ImpactHeavy},
		"lang-corepack":                 {model.TypeToolPrune, model.ImpactHeavy},
		"sys-nix-gc":                    {model.TypeToolPrune, model.ImpactHeavy},
		"desktop-flatpak-user":          {model.TypeToolPrune, model.ImpactHeavy},
		"desktop-flatpak-system":        {model.TypeToolPrune, model.ImpactHeavy},
		"lang-poetry-cache":             {model.TypeToolPrune, model.ImpactHeavy},
		"test-playwright-browsers":      {model.TypeToolPrune, model.ImpactHeavy},
		"desktop-download-partials":     {model.TypeDebris, model.ImpactSafe},
		"ai-huggingface-incomplete":     {model.TypeDebris, model.ImpactSafe},
		"python-ruff-cache":             {model.TypeDebris, model.ImpactSafe},
		"python-mypy-cache":             {model.TypeDebris, model.ImpactSafe},
		"python-pyinstaller-cache":      {model.TypeDebris, model.ImpactSafe},
		"gpu-nvidia-compute-cache":      {model.TypeDebris, model.ImpactSafe},
		"gpu-triton-cache":              {model.TypeDebris, model.ImpactSafe},
		"gpu-torch-inductor-cache":      {model.TypeDebris, model.ImpactSafe},
		"cloud-gcloud-logs":             {model.TypeDebris, model.ImpactSafe},
		"cloud-azure-logs":              {model.TypeDebris, model.ImpactSafe},
		"ai-huggingface-datasets-arrow": {model.TypeManagedCache, model.ImpactHeavy},
		"ai-torch-hub":                  {model.TypeManagedCache, model.ImpactHeavy},
	}
	loaded, err := registry.LoadBuiltin(cleaners.Builtin)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]model.Cleaner)
	for _, entry := range loaded {
		c := entry.Cleaner
		if c.Type != model.TypeToolPrune && c.Type != model.TypeDebris && c.Type != model.TypeManagedCache {
			continue
		}
		profile := c.Profile()
		if _, exists := seen[profile]; exists {
			t.Fatalf("duplicate closed profile %q", profile)
		}
		seen[profile] = c
		contract, ok := want[profile]
		if !ok {
			t.Fatalf("unexpected closed profile %q", profile)
		}
		if c.ID != profile {
			t.Errorf("profile %q cleaner ID = %q, want profile ID", profile, c.ID)
		}
		if c.Scope == "" || !c.IsEnabled() {
			t.Errorf("profile %q must be enabled with scope, got %+v", profile, c)
		}
		if c.Type != contract.typ || c.EffectiveImpact() != contract.impact {
			t.Errorf("profile %q type/impact = %q/%q, want %q/%q", profile, c.Type, c.EffectiveImpact(), contract.typ, contract.impact)
		}
		if c.MinAgeDays == nil || *c.MinAgeDays < 0 {
			t.Errorf("profile %q must declare non-negative min_age_days", profile)
		}
		if c.NoAgeFilter {
			t.Errorf("profile %q must not use no_age_filter", profile)
		}
	}
	for profile := range want {
		if _, ok := seen[profile]; !ok {
			t.Errorf("missing builtin profile %q", profile)
		}
	}
	for _, entry := range loaded {
		text := strings.ToLower(entry.Cleaner.ID + " " + entry.Cleaner.Name + " " + entry.Cleaner.Description)
		for _, forbidden := range []string{"github actions", "github-actions", "ollama", "modelscope", "macos"} {
			if strings.Contains(text, forbidden) {
				t.Errorf("builtin %q mentions out-of-scope cleaner %q", entry.Cleaner.ID, forbidden)
			}
		}
	}
}
