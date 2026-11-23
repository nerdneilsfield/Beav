package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/i18n"
)

// Options configures Plain and Spinner CLI event renderers.
// Options 配置 Plain 与 Spinner 命令行事件渲染器。
type Options struct {
	Translator *i18n.Translator
	Verbose    bool
	Animate    bool
}

type cleanerState struct {
	start       model.Event
	samples     []string
	sampleTotal int
	cmdLines    []string
	cmdTotal    int
}

type skipEntry struct {
	ID     string
	Name   string
	Detail string
}

const (
	verboseDetailLimit = 10
	previewCmdLimit    = 3
)

func resolveTranslator(tr *i18n.Translator) *i18n.Translator {
	if tr == nil {
		return i18n.New(i18n.LangEN)
	}
	return tr
}

func skipPhrase(tr *i18n.Translator, reason string) string {
	return resolveTranslator(tr).SkipPhrase(reason)
}

func skipRemedy(tr *i18n.Translator, reason string) string {
	return resolveTranslator(tr).SkipRemedy(reason)
}

func isQuietSkip(reason string) bool {
	return i18n.IsQuietSkip(reason)
}

func displayName(tr *i18n.Translator, id, rawName string) string {
	return resolveTranslator(tr).CleanerName(model.Cleaner{ID: id, Name: rawName})
}

func joinNames(tr *i18n.Translator, names []string, verbose bool) string {
	const max = 4
	sep := tr.Text(", ", "、")
	if verbose || len(names) <= max {
		return strings.Join(names, sep)
	}
	return strings.Join(names[:max], sep) + tr.Textf(", and %d more", "，另有 %d 项", len(names)-max)
}

func joinSkipEntries(tr *i18n.Translator, entries []skipEntry, verbose bool) string {
	const max = 6
	shown := entries
	if !verbose && len(entries) > max {
		shown = entries[:max]
	}
	parts := make([]string, 0, len(shown))
	for _, e := range shown {
		if e.ID != "" && e.Name != e.ID {
			parts = append(parts, fmt.Sprintf("%s (%s)", e.Name, e.ID))
		} else if e.ID != "" {
			parts = append(parts, e.ID)
		} else {
			parts = append(parts, e.Name)
		}
	}
	sep := tr.Text(", ", "、")
	out := strings.Join(parts, sep)
	if len(entries) > len(shown) {
		out += tr.Textf(", and %d more", "，另有 %d 项", len(entries)-len(shown))
	}
	return out
}

func sortedSkipReasons(m map[string][]skipEntry, quiet bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		if isQuietSkip(k) == quiet {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

func formatSummaryTail(tr *i18n.Translator, e model.Event) string {
	parts := make([]string, 0, 2)
	if e.CleanersSkipped > 0 {
		parts = append(parts, tr.Textf("%d skipped", "跳过 %d 项", e.CleanersSkipped))
	}
	if e.CleanersErrored > 0 {
		parts = append(parts, tr.Textf("%d errored", "出错 %d 项", e.CleanersErrored))
	}
	if len(parts) == 0 {
		return ""
	}
	if tr.IsZH() {
		return "（" + strings.Join(parts, "，") + "）"
	}
	return " (" + strings.Join(parts, ", ") + ")"
}
