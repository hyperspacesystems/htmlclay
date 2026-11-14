package dataguard_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/dataguard"
)

const typedQuestionsTag = `<script type="application/json" data-rules-name="api" data-rules-version="1">{questions:[".question",{key:".key",answer:".answer"}]}</script>`

type typedQuestion struct{ key, answer string }

func typedQuestionsHTML(qs ...typedQuestion) []byte {
	var b strings.Builder
	b.WriteString(`<html><body>`)
	for _, q := range qs {
		b.WriteString(`<div class="question"><span class="key">` + q.key + `</span>`)
		if q.answer != "" {
			b.WriteString(`<span class="answer">` + q.answer + `</span>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(typedQuestionsTag + `</body></html>`)
	return []byte(b.String())
}

const typedFieldsTag = `<script type="application/json" data-rules-name="api" data-rules-version="1">{f0:".f0", f1:".f1", f2:".f2", f3:".f3", f4:".f4", f5:".f5", f6:".f6", f7:".f7", f8:".f8", f9:".f9"}</script>`

func typedFieldsHTML(values ...string) []byte {
	var b strings.Builder
	b.WriteString(`<html><body>`)
	for i, v := range values {
		if v == "" {
			continue
		}
		cls := fmt.Sprintf("f%d", i)
		b.WriteString(`<span class="` + cls + `">` + v + `</span>`)
	}
	b.WriteString(typedFieldsTag + `</body></html>`)
	return []byte(b.String())
}

func (h *harness) observeFromPage(newHTML, prevHTML []byte, prov dataguard.Provenance, fromPage bool) *dataguard.ClientEvent {
	h.t.Helper()
	event, err := h.guard.Observe(dataguard.Observation{
		Key:      h.key,
		AbsPath:  h.docPath,
		NewHTML:  newHTML,
		PrevHTML: prevHTML,
		Prov:     prov,
		FromPage: fromPage,
		History:  h.history,
		Notify: func(kind string, event *dataguard.ClientEvent, id string) {
			h.notes = append(h.notes, notification{kind: kind, id: id, event: event})
		},
	})
	if err != nil {
		h.t.Fatalf("Observe: %v", err)
	}
	return event
}

func assertTypedAnswers(t *testing.T, label string, got []dataguard.TypedValue) {
	t.Helper()
	want := map[string]string{
		"api.questions[key:q1].answer": "one",
		"api.questions[key:q2].answer": "two",
		"api.questions[key:q3].answer": "three",
	}
	if len(got) != len(want) {
		t.Fatalf("%s: typed holds %+v, want the three answers", label, got)
	}
	for _, e := range got {
		preview, ok := want[e.Path]
		if !ok || e.Preview != preview {
			t.Fatalf("%s: unexpected typed value %+v", label, e)
		}
		if e.At != 1000 {
			t.Fatalf("%s: %s typed at %d, want 1000", label, e.Path, e.At)
		}
		delete(want, e.Path)
	}
}

// A tool rewrote an open form: it carried the person's three answers over and
// replaced one unanswered question. That must stay silent.
func TestObserveOutsideCarryingTypedAnswersStaysSilent(t *testing.T) {
	unanswered := typedQuestionsHTML(
		typedQuestion{key: "q1"}, typedQuestion{key: "q2"}, typedQuestion{key: "q3"}, typedQuestion{key: "q4"},
	)
	answered := typedQuestionsHTML(
		typedQuestion{key: "q1", answer: "one"},
		typedQuestion{key: "q2", answer: "two"},
		typedQuestion{key: "q3", answer: "three"},
		typedQuestion{key: "q4"},
	)
	rewritten := typedQuestionsHTML(
		typedQuestion{key: "q1", answer: "one"},
		typedQuestion{key: "q2", answer: "two"},
		typedQuestion{key: "q3", answer: "three"},
		typedQuestion{key: "q4", answer: "written by the tool"},
	)

	h := newHarness(t)
	if event := h.observe(unanswered, nil, dataguard.External); event != nil {
		t.Fatalf("cold seed fired: %+v", event)
	}
	if event := h.observe(answered, unanswered, dataguard.UIGestured); event != nil {
		t.Fatalf("the person's save fired: %+v", event)
	}
	h.clock = 2000
	if event := h.observe(rewritten, nil, dataguard.External); event != nil {
		t.Fatalf("a tool that carried the answers fired: %+v", event)
	}

	st := h.state()
	assertTypedAnswers(t, "typed after the outside write", st.Typed)
	if st.Baseline == nil || !bytes.Equal(st.Baseline.Data, islandJSON(t, rewritten)) {
		t.Fatalf("baseline did not follow the accepted write: %s", st.Baseline.Data)
	}
}

// A program that removes an answer the person typed minutes ago still raises.
func TestObserveOutsideRemovingRecentAnswerFires(t *testing.T) {
	result := func(t *testing.T, now int64) *dataguard.ClientEvent {
		t.Helper()
		twoAnswers := typedQuestionsHTML(
			typedQuestion{key: "q1", answer: "one"},
			typedQuestion{key: "q2", answer: "two"},
			typedQuestion{key: "q3"},
		)
		threeAnswers := typedQuestionsHTML(
			typedQuestion{key: "q1", answer: "one"},
			typedQuestion{key: "q2", answer: "two"},
			typedQuestion{key: "q3", answer: "three"},
		)
		removed := typedQuestionsHTML(
			typedQuestion{key: "q1", answer: "one"},
			typedQuestion{key: "q2", answer: "two"},
		)

		h := newHarness(t)
		h.clock = 500
		if event := h.observe(typedQuestionsHTML(typedQuestion{key: "q1"}, typedQuestion{key: "q2"}, typedQuestion{key: "q3"}), nil, dataguard.External); event != nil {
			t.Fatalf("cold seed fired: %+v", event)
		}
		h.clock = 1000
		if event := h.observe(twoAnswers, typedQuestionsHTML(typedQuestion{key: "q1"}, typedQuestion{key: "q2"}, typedQuestion{key: "q3"}), dataguard.UIGestured); event != nil {
			t.Fatalf("the person's first save fired: %+v", event)
		}
		h.clock = 2000
		if event := h.observe(threeAnswers, twoAnswers, dataguard.UIGestured); event != nil {
			t.Fatalf("the person's second save fired: %+v", event)
		}
		h.backupForTest(threeAnswers)
		h.clock = now
		h.backupForTest(removed)
		return h.observe(removed, nil, dataguard.External)
	}

	t.Run("within the window", func(t *testing.T) {
		event := result(t, 2000+2*60*1000)
		if event == nil {
			t.Fatal("removing a recent answer did not fire")
		}
		if event.Reason != "recent" {
			t.Fatalf("reason = %q, want recent", event.Reason)
		}
		if len(event.Typed) != 1 || event.Typed[0].Path != "api.questions[key:q3].answer" || event.Typed[0].Yours != "three" {
			t.Fatalf("client typed rows = %+v", event.Typed)
		}
		if event.Typed[0].At != 2000 {
			t.Fatalf("typed row time = %d, want 2000", event.Typed[0].At)
		}
	})

	t.Run("two hours later", func(t *testing.T) {
		if event := result(t, 2000+2*60*60*1000); event != nil {
			t.Fatalf("an old answer's removal fired: %+v", event)
		}
	})
}

// A page's own data API write keeps the old rules.
func TestObservePageDataWriteStillUsesShouldFire(t *testing.T) {
	threeAnswers := typedQuestionsHTML(
		typedQuestion{key: "q1", answer: "one"},
		typedQuestion{key: "q2", answer: "two"},
		typedQuestion{key: "q3", answer: "three"},
	)
	removed := typedQuestionsHTML(
		typedQuestion{key: "q1", answer: "one"},
		typedQuestion{key: "q2", answer: "two"},
	)

	h := newHarness(t)
	if event := h.observe(typedQuestionsHTML(typedQuestion{key: "q1"}, typedQuestion{key: "q2"}, typedQuestion{key: "q3"}), nil, dataguard.External); event != nil {
		t.Fatalf("cold seed fired: %+v", event)
	}
	if event := h.observe(threeAnswers, typedQuestionsHTML(typedQuestion{key: "q1"}, typedQuestion{key: "q2"}, typedQuestion{key: "q3"}), dataguard.UIGestured); event != nil {
		t.Fatalf("the person's save fired: %+v", event)
	}
	h.clock = 2000
	if event := h.observeFromPage(removed, nil, dataguard.External, true); event == nil {
		t.Fatal("a page removing an item did not fire")
	}
}

// An outside write that empties nearly all the data still speaks even with an
// empty typing record.
func TestObserveOutsideNearTotalWipeFires(t *testing.T) {
	ten := typedFieldsHTML("a", "b", "c", "d", "e", "f", "g", "h", "i", "j")
	one := typedFieldsHTML("a")

	h := newHarness(t)
	if event := h.observe(ten, nil, dataguard.External); event != nil {
		t.Fatalf("cold seed fired: %+v", event)
	}
	event := h.observe(one, nil, dataguard.External)
	if event == nil {
		t.Fatal("an outside write that emptied nine of ten values did not fire")
	}
	if event.Reason != "emptied" {
		t.Fatalf("reason = %q, want emptied", event.Reason)
	}
}
