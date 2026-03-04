package container

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDetect_GoRepo(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/test\n\ngo 1.22.1\n")

	langs := Detect(context.Background(), dir)

	if len(langs) != 1 {
		t.Fatalf("expected 1 language, got %d", len(langs))
	}
	if langs[0].Language != LangGo {
		t.Errorf("expected Go, got %s", langs[0].Language)
	}
	if langs[0].Version != "1.22" {
		t.Errorf("expected version 1.22, got %s", langs[0].Version)
	}
}

func TestDetect_NodeRepo(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"name":"test"}`)

	langs := Detect(context.Background(), dir)

	if len(langs) != 1 {
		t.Fatalf("expected 1 language, got %d", len(langs))
	}
	if langs[0].Language != LangNode {
		t.Errorf("expected Node, got %s", langs[0].Language)
	}
}

func TestDetect_NodeWithNvmrc(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"name":"test"}`)
	writeFile(t, dir, ".nvmrc", "v20.11.0\n")

	langs := Detect(context.Background(), dir)

	if len(langs) != 1 {
		t.Fatalf("expected 1 language, got %d", len(langs))
	}
	if langs[0].Version != "20" {
		t.Errorf("expected version 20, got %s", langs[0].Version)
	}
}

func TestDetect_NodeWithNodeVersion(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"name":"test"}`)
	writeFile(t, dir, ".node-version", "18.17.0\n")

	langs := Detect(context.Background(), dir)

	if len(langs) != 1 {
		t.Fatalf("expected 1 language, got %d", len(langs))
	}
	if langs[0].Version != "18" {
		t.Errorf("expected version 18, got %s", langs[0].Version)
	}
}

func TestDetect_PythonRepo(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "requirements.txt", "flask==2.0\n")

	langs := Detect(context.Background(), dir)

	if len(langs) != 1 {
		t.Fatalf("expected 1 language, got %d", len(langs))
	}
	if langs[0].Language != LangPython {
		t.Errorf("expected Python, got %s", langs[0].Language)
	}
}

func TestDetect_PythonWithVersion(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pyproject.toml", "[tool.poetry]\nname = \"test\"\n")
	writeFile(t, dir, ".python-version", "3.12.1\n")

	langs := Detect(context.Background(), dir)

	if len(langs) != 1 {
		t.Fatalf("expected 1 language, got %d", len(langs))
	}
	if langs[0].Version != "3.12" {
		t.Errorf("expected version 3.12, got %s", langs[0].Version)
	}
}

func TestDetect_RubyRepo(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Gemfile", "source 'https://rubygems.org'\n")

	langs := Detect(context.Background(), dir)

	if len(langs) != 1 {
		t.Fatalf("expected 1 language, got %d", len(langs))
	}
	if langs[0].Language != LangRuby {
		t.Errorf("expected Ruby, got %s", langs[0].Language)
	}
}

func TestDetect_RubyWithVersion(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Gemfile", "source 'https://rubygems.org'\n")
	writeFile(t, dir, ".ruby-version", "ruby-3.2.0\n")

	langs := Detect(context.Background(), dir)

	if len(langs) != 1 {
		t.Fatalf("expected 1 language, got %d", len(langs))
	}
	if langs[0].Version != "3.2" {
		t.Errorf("expected version 3.2, got %s", langs[0].Version)
	}
}

func TestDetect_RustRepo(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Cargo.toml", "[package]\nname = \"test\"\n")

	langs := Detect(context.Background(), dir)

	if len(langs) != 1 {
		t.Fatalf("expected 1 language, got %d", len(langs))
	}
	if langs[0].Language != LangRust {
		t.Errorf("expected Rust, got %s", langs[0].Language)
	}
}

func TestDetect_JavaRepo(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pom.xml", "<project></project>")

	langs := Detect(context.Background(), dir)

	if len(langs) != 1 {
		t.Fatalf("expected 1 language, got %d", len(langs))
	}
	if langs[0].Language != LangJava {
		t.Errorf("expected Java, got %s", langs[0].Language)
	}
}

func TestDetect_MultiLanguage(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module test\n\ngo 1.22\n")
	writeFile(t, dir, "package.json", `{"name":"test"}`)
	writeFile(t, dir, "requirements.txt", "flask\n")

	langs := Detect(context.Background(), dir)

	if len(langs) != 3 {
		t.Fatalf("expected 3 languages, got %d", len(langs))
	}

	// Verify deterministic sort order: Go, Node, Python
	if langs[0].Language != LangGo {
		t.Errorf("expected Go first, got %s", langs[0].Language)
	}
	if langs[1].Language != LangNode {
		t.Errorf("expected Node second, got %s", langs[1].Language)
	}
	if langs[2].Language != LangPython {
		t.Errorf("expected Python third, got %s", langs[2].Language)
	}
}

func TestDetect_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	langs := Detect(context.Background(), dir)

	if len(langs) != 0 {
		t.Errorf("expected 0 languages, got %d", len(langs))
	}
}

func TestDetect_DuplicateLanguageMarkers(t *testing.T) {
	dir := t.TempDir()
	// Multiple Python markers should only detect Python once
	writeFile(t, dir, "requirements.txt", "flask\n")
	writeFile(t, dir, "pyproject.toml", "[tool]\n")
	writeFile(t, dir, "setup.py", "from setuptools import setup\n")

	langs := Detect(context.Background(), dir)

	if len(langs) != 1 {
		t.Fatalf("expected 1 language (deduped), got %d", len(langs))
	}
	if langs[0].Language != LangPython {
		t.Errorf("expected Python, got %s", langs[0].Language)
	}
}

func TestDetect_CancelledContext(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module test\n\ngo 1.22\n")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	langs := Detect(ctx, dir)

	// Should return early (may have 0 or some results depending on timing)
	_ = langs
}

func TestDetect_NonexistentDir(t *testing.T) {
	langs := Detect(context.Background(), "/nonexistent/path")

	if len(langs) != 0 {
		t.Errorf("expected 0 languages for nonexistent path, got %d", len(langs))
	}
}

func TestDetectGoVersion_MajorMinor(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module test\n\ngo 1.22\n")

	v := detectGoVersion(dir)
	if v != "1.22" {
		t.Errorf("expected 1.22, got %s", v)
	}
}

func TestDetectGoVersion_MajorMinorPatch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module test\n\ngo 1.25.4\n")

	v := detectGoVersion(dir)
	if v != "1.25" {
		t.Errorf("expected 1.25, got %s", v)
	}
}

func TestDetectGoVersion_NoGoMod(t *testing.T) {
	dir := t.TempDir()

	v := detectGoVersion(dir)
	if v != "" {
		t.Errorf("expected empty, got %s", v)
	}
}

func TestDetectGoVersion_NoGoDirective(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module test\n")

	v := detectGoVersion(dir)
	if v != "" {
		t.Errorf("expected empty, got %s", v)
	}
}

func TestDetect_JavaGradle(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "build.gradle", "plugins { id 'java' }\n")

	langs := Detect(context.Background(), dir)

	if len(langs) != 1 {
		t.Fatalf("expected 1 language, got %d", len(langs))
	}
	if langs[0].Language != LangJava {
		t.Errorf("expected Java, got %s", langs[0].Language)
	}
}

func TestDetect_DeterministicOrder(t *testing.T) {
	dir := t.TempDir()
	// Create files in reverse order to test sorting
	writeFile(t, dir, "Cargo.toml", "[package]\n")
	writeFile(t, dir, "Gemfile", "source 'https://rubygems.org'\n")
	writeFile(t, dir, "go.mod", "module test\n\ngo 1.22\n")

	langs := Detect(context.Background(), dir)

	if len(langs) != 3 {
		t.Fatalf("expected 3 languages, got %d", len(langs))
	}
	// Go (0) < Ruby (3) < Rust (4)
	if langs[0].Language != LangGo {
		t.Errorf("expected Go first, got %s", langs[0].Language)
	}
	if langs[1].Language != LangRuby {
		t.Errorf("expected Ruby second, got %s", langs[1].Language)
	}
	if langs[2].Language != LangRust {
		t.Errorf("expected Rust third, got %s", langs[2].Language)
	}
}

// writeFile creates a file in the given directory with the given content.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
