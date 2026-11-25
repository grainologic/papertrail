package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"papertrail/internal/item"
	"papertrail/internal/template"
)

// writeItems writes n linked items (a dependency chain) directly, without the CLI, so the benchmark measures one sync pass only.
func writeItems(b *testing.B, root string, n int) {
	b.Helper()
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("%08d", i)
		title := fmt.Sprintf("bench item %d", i)
		fm := item.Frontmatter{
			ID:       id,
			Title:    title,
			Type:     "task",
			Status:   "open",
			Priority: 2,
			Mode:     item.ModeAuto,
			Created:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		}
		if i+1 < n {
			fm.DependsOn = []string{fmt.Sprintf("%08d", i+1)}
		}
		body, err := template.Body("task", title)
		if err != nil {
			b.Fatal(err)
		}
		dir := filepath.Join(root, id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			b.Fatal(err)
		}
		content, err := fm.Encode([]byte(body))
		if err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, title+".md"), content, 0o644); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSync1000 measures a full sync pass over 1,000 linked items, cold (no cache) and warm (cache from the previous pass).
func BenchmarkSync1000(b *testing.B) {
	root := b.TempDir()
	writeItems(b, root, 1000)
	if err := os.Remove(filepath.Join(root, "cache.json")); err != nil && !os.IsNotExist(err) {
		b.Fatal(err)
	}
	b.Run("cold", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			if err := os.Remove(filepath.Join(root, "cache.json")); err != nil && !os.IsNotExist(err) {
				b.Fatal(err)
			}
			b.StartTimer()
			if _, err := Run(root); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("warm", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err := Run(root); err != nil {
				b.Fatal(err)
			}
		}
	})
}
