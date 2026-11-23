package i18n

import (
	"strings"
	"testing"

	"github.com/dengqi/beav/cleaners"
	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/cleaner/registry"
)

func TestParseLang(t *testing.T) {
	cases := []struct {
		in   string
		want Lang
		ok   bool
	}{
		{"", "", true},
		{"auto", "", true},
		{"en", LangEN, true},
		{"en_US.UTF-8", LangEN, true},
		{"zh", LangZH, true},
		{"zh_CN.UTF-8", LangZH, true},
		{"zh-Hans", LangZH, true},
		{"fr_FR", "", false},
	}
	for _, tc := range cases {
		got, ok := ParseLang(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("ParseLang(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestDetectFromPrecedence(t *testing.T) {
	env := func(values map[string]string) func(string) string {
		return func(k string) string { return values[k] }
	}

	if got := DetectFrom("zh", env(map[string]string{"LANG": "en_US.UTF-8"})); got != LangZH {
		t.Fatalf("flag should win over env, got %q", got)
	}
	if got := DetectFrom("auto", env(map[string]string{"BEAV_LANG": "zh", "LANG": "en_US.UTF-8"})); got != LangZH {
		t.Fatalf("BEAV_LANG should win over LANG, got %q", got)
	}
	if got := DetectFrom("", env(map[string]string{"LC_ALL": "C", "LANG": "zh_CN.UTF-8"})); got != LangEN {
		t.Fatalf("LC_ALL=C should select EN, got %q", got)
	}
	if got := DetectFrom("", env(map[string]string{"LANG": "zh_TW.UTF-8"})); got != LangZH {
		t.Fatalf("LANG=zh_TW.UTF-8 should select ZH, got %q", got)
	}
	if got := DetectFrom("", env(map[string]string{})); got != LangEN {
		t.Fatalf("empty env should default to EN, got %q", got)
	}
}

func TestDetectFromArgs(t *testing.T) {
	env := func(string) string { return "en_US.UTF-8" }
	if got := DetectFromArgs([]string{"clean", "--lang", "zh", "--help"}, env); got != LangZH {
		t.Fatalf("DetectFromArgs space form = %q, want zh", got)
	}
	if got := DetectFromArgs([]string{"--lang=zh", "list"}, env); got != LangZH {
		t.Fatalf("DetectFromArgs equals form = %q, want zh", got)
	}
	if got := DetectFromArgs([]string{"--", "--lang=zh"}, env); got != LangEN {
		t.Fatalf("DetectFromArgs must stop at --, got %q", got)
	}
}

func TestSkipRemedyAccuracy(t *testing.T) {
	for _, lang := range []Lang{LangEN, LangZH} {
		tr := New(lang)
		if remedy := tr.SkipRemedy("root_unavailable"); remedy != "" || strings.Contains(strings.ToLower(remedy), "sudo") {
			t.Fatalf("[%s] root_unavailable remedy must be empty and never suggest sudo, got %q", lang, remedy)
		}
		if remedy := tr.SkipRemedy(string(model.SkipReasonManagerNotInstalled)); remedy != "" {
			t.Fatalf("[%s] manager_not_installed remedy must be empty, got %q", lang, remedy)
		}
		if remedy := tr.SkipRemedy(string(model.SkipReasonRuntimeUnavailable)); remedy != "" {
			t.Fatalf("[%s] runtime_unavailable remedy must be empty, got %q", lang, remedy)
		}
		if phrase := tr.SkipPhrase("root_unavailable"); phrase == "" {
			t.Fatalf("[%s] root_unavailable phrase missing", lang)
		}
	}
}

func TestAllBuiltinCleanersHaveBilingualMetadata(t *testing.T) {
	loaded, err := registry.LoadBuiltin(cleaners.Builtin)
	if err != nil {
		t.Fatalf("load builtin: %v", err)
	}
	en := New(LangEN)
	zh := New(LangZH)
	for _, l := range loaded {
		c := l.Cleaner
		if en.CleanerName(c) == "" || zh.CleanerName(c) == "" {
			t.Errorf("cleaner %q missing localized name", c.ID)
		}
		if en.CleanerDesc(c) == "" {
			t.Errorf("cleaner %q missing EN description", c.ID)
		}
		if zh.CleanerDesc(c) == "" {
			t.Errorf("cleaner %q missing ZH description", c.ID)
		}
		if c.EffectiveImpact() == model.ImpactHeavy {
			if en.HeavyNote(c) == "" || zh.HeavyNote(c) == "" {
				t.Errorf("heavy cleaner %q missing localized heavy note", c.ID)
			}
		}
	}
}

func TestPlural(t *testing.T) {
	en := New(LangEN)
	zh := New(LangZH)
	if got := en.Plural(1, "file", "files", "个文件"); got != "1 file" {
		t.Fatalf("en singular = %q", got)
	}
	if got := en.Plural(3, "file", "files", "个文件"); got != "3 files" {
		t.Fatalf("en plural = %q", got)
	}
	if got := zh.Plural(1, "file", "files", "个文件"); got != "1 个文件" {
		t.Fatalf("zh = %q", got)
	}
}

func TestImpactExecutorAndTimeFieldLabels(t *testing.T) {
	en := New(LangEN)
	zh := New(LangZH)

	if got := en.ImpactLabel(model.ImpactSafe); got != "safe" {
		t.Fatalf("en safe = %q", got)
	}
	if got := en.ImpactLabel(model.ImpactHeavy); got != "heavy" {
		t.Fatalf("en heavy = %q", got)
	}
	if got := zh.ImpactLabel(model.ImpactSafe); got != "常规" {
		t.Fatalf("zh safe = %q, want 常规", got)
	}
	if got := zh.ImpactLabel(model.ImpactHeavy); got != "重度" {
		t.Fatalf("zh heavy = %q, want 重度", got)
	}

	types := []model.ExecutorType{
		model.TypePaths,
		model.TypePkgCache,
		model.TypeJournalVacuum,
		model.TypeContainerPrune,
		model.TypeToolPrune,
		model.TypeDebris,
		model.TypeManagedCache,
		model.TypeRustupPrune,
	}
	for _, et := range types {
		if got := en.ExecutorTypeLabel(et); got == "" || got == string(et) {
			t.Fatalf("en ExecutorTypeLabel(%q) = %q, want human label", et, got)
		}
		if got := zh.ExecutorTypeLabel(et); got == "" || got == string(et) {
			t.Fatalf("zh ExecutorTypeLabel(%q) = %q, want Chinese label", et, got)
		}
	}

	if got := zh.TimeFieldLabel(model.TimeMtime); got != "修改时间 mtime" {
		t.Fatalf("zh mtime = %q", got)
	}
	if got := zh.TimeFieldLabel(model.TimeCtime); got != "创建时间 ctime" {
		t.Fatalf("zh ctime = %q", got)
	}
}
