package scripting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectCategories_Breakpoint(t *testing.T) {
	src := `function onRequest(ctx) { ctx.breakpoint(); }`
	cats := DetectCategories(src)
	assert.Equal(t, []ScriptCategory{CategoryBreakpoint}, cats)
}

func TestDetectCategories_MapLocal(t *testing.T) {
	src := `function onRequest(ctx) { ctx.respondWith({file: "f.json"}); }`
	cats := DetectCategories(src)
	assert.Equal(t, []ScriptCategory{CategoryMapLocal}, cats)
}

func TestDetectCategories_Both(t *testing.T) {
	src := `function onRequest(ctx) {
		ctx.breakpoint();
		ctx.respondWith({body: "x"});
	}`
	cats := DetectCategories(src)
	assert.Equal(t, []ScriptCategory{CategoryBreakpoint, CategoryMapLocal}, cats)
}

func TestDetectCategories_Default(t *testing.T) {
	src := `function onRequest(ctx) { ctx.headers["X"] = "y"; }`
	cats := DetectCategories(src)
	assert.Equal(t, []ScriptCategory{CategoryScript}, cats)
}

func TestDetectCategories_Empty(t *testing.T) {
	cats := DetectCategories("")
	assert.Equal(t, []ScriptCategory{CategoryScript}, cats)
}

func TestScriptInfos_IncludesCategories(t *testing.T) {
	dir := t.TempDir()
	writeTestScript(t, dir, "bp.js", `// ---
// name: Breakpoint Script
// match:
//   - "*://*/*"
// ---
function onRequest(ctx) {
    ctx.breakpoint();
}
`)
	writeTestScript(t, dir, "mock.js", `// ---
// name: Mock Script
// match:
//   - "*://*/*"
// ---
function onRequest(ctx) {
    ctx.respondWith({file: "./data.json"});
}
`)
	writeTestScript(t, dir, "plain.js", `// ---
// name: Plain Script
// match:
//   - "*://*/*"
// ---
function onRequest(ctx) {
    ctx.headers["X-Test"] = "yes";
}
`)

	e := New()
	e.LoadFromDir(dir)

	infos := e.ScriptInfos()
	assert.Len(t, infos, 3)

	catMap := map[string][]ScriptCategory{}
	for _, info := range infos {
		catMap[info.Name] = info.Categories
	}

	assert.Equal(t, []ScriptCategory{CategoryBreakpoint}, catMap["Breakpoint Script"])
	assert.Equal(t, []ScriptCategory{CategoryMapLocal}, catMap["Mock Script"])
	assert.Equal(t, []ScriptCategory{CategoryScript}, catMap["Plain Script"])
}
