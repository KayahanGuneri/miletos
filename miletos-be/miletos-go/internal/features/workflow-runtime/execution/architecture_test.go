package execution

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestGenericExecutionPackagesContainNoConcretePluginKnowledge(t *testing.T) {
	genericPackages := []string{".", "../workflow"}
	for _, root := range genericPackages {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			text := string(content)
			for _, pluginName := range []string{
				"http-trigger", "static-input", "pass-through", "delay", "join", "terminal",
			} {
				forbidden := "core" + "." + pluginName
				if strings.Contains(text, forbidden) {
					t.Errorf("generic package %s contains concrete plugin knowledge %q", path, forbidden)
				}
			}
			for _, forbidden := range []string{
				`Configuration["method"]`, `Configuration["url"]`,
				`Configuration["value"]`, "internal/features/workflowruntime",
			} {
				if strings.Contains(text, forbidden) {
					t.Errorf("generic package %s contains concrete plugin knowledge %q", path, forbidden)
				}
			}
			for _, pattern := range []string{
				`\bnode\s*\.\s*Type\s*==`,
				`switch\s+node\s*\.\s*Type`,
			} {
				if regexp.MustCompile(pattern).MatchString(text) {
					t.Errorf("generic package %s contains concrete plugin branch %q", path, pattern)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan generic package %s: %v", root, err)
		}
	}
}

func TestGenericControllerDoesNotOwnRepositoriesOrCommandConstruction(t *testing.T) {
	content, err := os.ReadFile("controller.go")
	if err != nil {
		t.Fatalf("read controller.go: %v", err)
	}
	text := string(content)
	for _, forbidden := range []string{
		"repository.", "*repository.", "NewExecutionCommand", "Fingerprint(",
	} {
		if strings.Contains(text, forbidden) {
			t.Errorf("generic controller contains forbidden orchestration %q", forbidden)
		}
	}
}
