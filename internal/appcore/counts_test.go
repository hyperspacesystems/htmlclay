package appcore

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestCountsEachKind(t *testing.T) {
	f := Facts{
		Home: "/Users/sam",
		Host: HostFacts{Service: ServiceFailed},
		Events: []EventFact{
			{ID: "c1", Kind: WaitingConflict, SubjectID: "team", Name: "a.html", Detail: "Team"},
			{ID: "k1", Kind: WaitingSyncKeyRefused, SubjectID: "f1"},
			{ID: "k2", Kind: WaitingSyncKeyRefused, SubjectID: "f2"},
			{ID: "k3", Kind: WaitingSyncKeyRefused, SubjectID: "f3"},
			{ID: "big", Kind: WaitingFileTooBig, Name: "movie.mov"},
			{ID: "team", Kind: WaitingTeamToSetUp, Name: "Marketing"},
		},
		Requests: []RequestFact{{ID: "r1", Kind: RequestFound, Document: "todo.htmlclay", Program: "search"}},
		Programs: []Program{{ID: "p1", Name: "search", Path: "/Users/sam/bin/search", FileMissing: true}},
		Folders: []FolderFact{
			{ID: "gone", Name: "Gone", Path: "/Users/sam/Gone", OnThisComputer: true, Trusted: true, Missing: true},
			{ID: "away", Name: "Away", OnThisComputer: false},
		},
	}
	s := Build(f)

	kinds := map[string]int{}
	for _, item := range s.Waiting {
		kinds[item.Kind]++
		if item.Title == "" || strings.Contains(item.Title, "%") {
			t.Errorf("item %s has title %q", item.ID, item.Title)
		}
	}
	want := map[string]int{
		WaitingConflict: 1, WaitingSyncKeyRefused: 1, WaitingFileTooBig: 1, WaitingTeamToSetUp: 0,
		WaitingProgramRequest: 1, WaitingProgramFileMissing: 1, WaitingFolderMissing: 1, WaitingServiceFailed: 1,
	}
	for kind, n := range want {
		if kinds[kind] != n {
			t.Errorf("%d %s items, want %d", kinds[kind], kind, n)
		}
	}
	// Six of the seven are counted: a file too big is listed only; un-set-up teams do not appear in Waiting.
	if s.Counts.Total != 6 {
		t.Errorf("total = %d, want 6 (a refused key counts once)", s.Counts.Total)
	}
	wantPanes := map[string]int{PaneFolders: 2, PanePrograms: 2, PaneSync: 1, PaneAdvanced: 1}
	sum := 0
	for pane, n := range wantPanes {
		if s.Counts.ByPane[pane] != n {
			t.Errorf("pane %s = %d, want %d", pane, s.Counts.ByPane[pane], n)
		}
		sum += s.Counts.ByPane[pane]
	}
	if sum != s.Counts.Total {
		t.Errorf("panes sum to %d, total is %d", sum, s.Counts.Total)
	}
	if !s.Counts.Dot {
		t.Error("dot is off with unseen items that earn it")
	}
}

func TestCountsDotClearsWhenSeen(t *testing.T) {
	f := Facts{
		Requests: []RequestFact{{ID: "r1", Kind: RequestFound, Document: "d", Program: "p"}},
		Events:   []EventFact{{ID: "t1", Kind: WaitingTeamToSetUp, Name: "Team"}},
	}
	if !Build(f).Counts.Dot {
		t.Fatal("dot is off before anything is seen")
	}
	f.Seen = []string{"w-r1"}
	s := Build(f)
	if s.Counts.Dot {
		t.Error("dot stays on after the only item that earns it was seen")
	}
	if s.Counts.Total != 1 {
		t.Errorf("seeing an item changed the count to %d; seen clears the dot, not the count", s.Counts.Total)
	}
	if len(s.Waiting) != 1 || !s.Waiting[0].Seen {
		t.Errorf("waiting = %+v, want only the seen request (no teamToSetUp item)", s.Waiting)
	}
}

func TestCountsEmpty(t *testing.T) {
	s := Build(Facts{})
	if s.Counts.Total != 0 || s.Counts.Dot || len(s.Counts.ByPane) != len(countedPanes) {
		t.Errorf("empty counts = %+v", s.Counts)
	}
}

func TestCountsFolderReadTitles(t *testing.T) {
	f := Facts{Requests: []RequestFact{
		{ID: "a", Kind: RequestFolderRead, Document: "budget.htmlclay", Folder: "Receipts"},
		{ID: "b", Kind: RequestFolderReadAgain, Document: "trip.htmlclay", Folder: "Trips"},
	}}
	s := Build(f)
	if !strings.Contains(s.Waiting[0].Title, "Receipts") || !strings.Contains(s.Waiting[1].Title, "Trips") {
		t.Errorf("folder request titles = %q, %q", s.Waiting[0].Title, s.Waiting[1].Title)
	}
}

func TestCountsAIEditTitles(t *testing.T) {
	f := Facts{Requests: []RequestFact{
		{ID: "a", Kind: RequestTeamDocument, Document: "roadmap.htmlclay", Program: AIEditProgram, Folder: "Design"},
		{ID: "b", Kind: RequestTeamDocument, Document: "roadmap.htmlclay", Program: "todo-search", Folder: "Design"},
	}}
	s := Build(f)
	if want := fmt.Sprintf(TextWaitingAIEditRequest, "roadmap.htmlclay"); s.Waiting[0].Title != want {
		t.Errorf("AI editing request title = %q, want %q", s.Waiting[0].Title, want)
	}
	if want := fmt.Sprintf(TextWaitingProgramRequest, "roadmap.htmlclay", "todo-search"); s.Waiting[1].Title != want {
		t.Errorf("program request title = %q, want %q", s.Waiting[1].Title, want)
	}
}

func TestCountsUnknownDocumentTitleNamesTheFolder(t *testing.T) {
	s := Build(Facts{Requests: []RequestFact{
		{ID: "a", Kind: RequestUnknownDocument, Document: "/Users/me/Downloads/invoice.html", Folder: "Downloads"},
	}})
	if title := s.Waiting[0].Title; !strings.Contains(title, "invoice.html") || !strings.Contains(title, "Downloads") {
		t.Errorf("unknown document title = %q, want the document and the folder it can reach", title)
	}
	if got := s.Requests[0].Answers; !slices.Equal(got, []string{"deny-document", "allow-document", "trust-document"}) {
		t.Errorf("unknown document answers = %v", got)
	}
}

func TestCountsBuiltinProgramTitle(t *testing.T) {
	s := Build(Facts{Requests: []RequestFact{
		{ID: "a", Kind: RequestBuiltinProgram, Document: "todo.htmlclay", Program: "file-search", Folder: "Notes"},
	}})
	if title := s.Waiting[0].Title; !strings.Contains(title, "File search") || strings.Contains(title, "file-search") {
		t.Errorf("built-in program request title = %q, want the name a person reads", title)
	}
}
