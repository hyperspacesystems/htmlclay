package htmlutil

import (
	"fmt"
	"strings"
	"testing"
)

func programTag(name, can, text string) string {
	return fmt.Sprintf(`<script type="text/x-clay-program" name=%q can=%q>%s</script>`, name, can, text)
}

func TestThreeLargeProgramsAreAllRead(t *testing.T) {
	var b strings.Builder
	b.WriteString("<!doctype html><html><head><title>t</title></head><body>")
	for i := range 3 {
		body := "async function main() { return 1 }\n//" + strings.Repeat("x", 200<<10)
		b.WriteString(programTag(fmt.Sprintf("p%d", i), "read", body))
	}
	b.WriteString("</body></html>")
	scan := ReadPrograms([]byte(b.String()))
	if len(scan.Programs) != 3 || len(scan.NotRead) != 0 {
		t.Fatalf("read %d programs, not read %v; want all three read", len(scan.Programs), scan.NotRead)
	}
	for i, p := range scan.Programs {
		if p.Name != fmt.Sprintf("p%d", i) || p.Can != "read" || !strings.HasPrefix(p.Text, "async function main()") || len(p.Text) < 200<<10 {
			t.Fatalf("program %d = %q can %q, %d bytes", i, p.Name, p.Can, len(p.Text))
		}
	}
}

func TestAProgramCutByTheLimitIsReportedNotDropped(t *testing.T) {
	pad := strings.Repeat("<p>filler</p>", (ProgramScanLimit-200<<10)/len("<p>filler</p>"))
	doc := "<html><body>" + pad +
		programTag("across", "", strings.Repeat("y", 250<<10)) +
		strings.Repeat("<p>filler</p>", 30000) +
		programTag("after", "", "async function main() {}") + "</body></html>"
	scan := ReadPrograms([]byte(doc))
	if len(scan.Programs) != 0 {
		t.Fatalf("read %v, want nothing read", scan.Programs)
	}
	if r, ok := scan.Reason("across"); !ok || r != NotReadCut {
		t.Fatalf("across: %q %v, want %q", r, ok, NotReadCut)
	}
	if r, ok := scan.Reason("after"); !ok || r != NotReadPastLimit {
		t.Fatalf("after: %q %v, want %q", r, ok, NotReadPastLimit)
	}
}

func TestAtMostEightProgramsOf256KiB(t *testing.T) {
	var b strings.Builder
	b.WriteString(programTag("big", "", strings.Repeat("z", MaxProgramBytes+1)))
	for i := range 9 {
		b.WriteString(programTag(fmt.Sprintf("n%d", i), "", "async function main() {}"))
	}
	b.WriteString(programTag("Bad Name", "", ""))
	scan := ReadPrograms([]byte(b.String()))
	if len(scan.Programs) != MaxPrograms {
		t.Fatalf("read %d programs, want %d", len(scan.Programs), MaxPrograms)
	}
	for name, want := range map[string]string{"big": NotReadTooLarge, "n8": NotReadTooMany, "Bad Name": NotReadBadName} {
		if r, ok := scan.Reason(name); !ok || r != want {
			t.Fatalf("%s: %q %v, want %q", name, r, ok, want)
		}
	}
}

func TestTwoProgramsWithOneNameAreBothRefused(t *testing.T) {
	scan := ReadPrograms([]byte(programTag("notes", "", "async function main() { return 1 }") +
		programTag("fmt", "", "") + programTag("notes", "", "async function main() { return 2 }")))
	if p, ok := scan.Program("notes"); ok {
		t.Fatalf("notes = %q: a name two tags share was read, so one of them would run unseen", p.Text)
	}
	if r, ok := scan.Reason("notes"); !ok || r != NotReadDuplicate {
		t.Fatalf("notes: %q %v, want %q", r, ok, NotReadDuplicate)
	}
	if _, ok := scan.Program("fmt"); !ok || len(scan.NotRead) != 1 {
		t.Fatalf("programs %v, not read %v", scan.Programs, scan.NotRead)
	}
}

func TestProgramTextIsTheFilesOwnAndTemplatesHoldNone(t *testing.T) {
	text := "async function main() { return \"<b>&amp;</b>\" + '<\\/script>' }"
	doc := `<html><head><SCRIPT TYPE=" Text/X-Clay-Program " name="one" can="read git">` + text + `</SCRIPT></head>` +
		`<body><template>` + programTag("hidden", "", "x") + `</template>` +
		`<script type="text/x-clay-program" name="empty"></script><p>after</p></body></html>`
	scan := ReadPrograms([]byte(doc))
	if len(scan.Programs) != 2 {
		t.Fatalf("read %v, want one and empty", scan.Programs)
	}
	if p := scan.Programs[0]; p.Name != "one" || p.Can != "read git" || p.Text != text {
		t.Fatalf("one = %+v, want the text as written", p)
	}
	if _, ok := scan.Program("hidden"); ok {
		t.Fatal("a program inside a template was read")
	}
}

func TestForeignNamespacesHoldNoPrograms(t *testing.T) {
	for _, doc := range []string{
		`<html><body><svg><![CDATA[><script type="text/x-clay-program" name="pwn" can="change">async function main() {}</script>]]></svg></body></html>`,
		`<html><body><svg><script type="text/x-clay-program" name="svgp" can="change">async function main() {}</script></svg></body></html>`,
		`<html><body><math><script type="text/x-clay-program" name="mathp">async function main() {}</script></math></body></html>`,
	} {
		if scan := ReadPrograms([]byte(doc)); len(scan.Programs) != 0 {
			t.Fatalf("read %v from %q, want nothing: the browser does not run that tag", scan.Programs, doc)
		}
	}
}

func TestAFramesetDropsItsProgram(t *testing.T) {
	doc := `<html><frameset><script type="text/x-clay-program" name="fs" can="change">async function main() {}</script></frameset></html>`
	if scan := ReadPrograms([]byte(doc)); len(scan.Programs) != 0 {
		t.Fatalf("read %v, want nothing: the parser drops a frameset's script", scan.Programs)
	}
}

func TestASelfClosingTemplateHidesTheProgramAfterIt(t *testing.T) {
	doc := `<html><body><template /><script type="text/x-clay-program" name="tpl" can="change">async function main() {}</script></template></body></html>`
	if scan := ReadPrograms([]byte(doc)); len(scan.Programs) != 0 {
		t.Fatalf("read %v, want nothing: a self-closing template still holds what follows", scan.Programs)
	}
}

func TestReservedNamesAreNotPrograms(t *testing.T) {
	for _, name := range []string{"ai-tools", "ai-edit", "programs", "document-trust"} {
		scan := ReadPrograms([]byte(programTag(name, "read", "async function main() {}")))
		if len(scan.Programs) != 0 {
			t.Fatalf("read %v, want nothing: %q is a reserved name", scan.Programs, name)
		}
		if r, ok := scan.Reason(name); !ok || r != NotReadBadName {
			t.Fatalf("%s: %q %v, want %q", name, r, ok, NotReadBadName)
		}
	}
}
