package appcore

import (
	"errors"
	"testing"

	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/pathrel"
)

func TestCheckServe(t *testing.T) {
	const (
		site      = "/home/u/Sites/site"
		served    = "/home/u/Sites/served"
		trusted   = "/home/u/Sites/trusted"
		configDir = "/home/u/.config/htmlclay"
	)
	allowed := ServeFacts{
		Path:   site,
		Exists: true,
		IsDir:  true,
		Home:   pathrel.Inside,
		Protected: []OtherFolder{
			{Relation: pathrel.Unrelated, Path: configDir},
		},
		Served: []OtherFolder{
			{Relation: pathrel.Unrelated, Path: served, Name: "served"},
		},
		Trusted: []OtherFolder{
			{Relation: pathrel.Unrelated, Path: trusted},
		},
		LocalServed: []OtherFolder{
			{Relation: pathrel.Unrelated, Path: "/home/u/Local"},
		},
	}
	with := func(change func(*ServeFacts)) ServeFacts {
		facts := allowed
		facts.Protected = append([]OtherFolder(nil), allowed.Protected...)
		facts.Served = append([]OtherFolder(nil), allowed.Served...)
		facts.Trusted = append([]OtherFolder(nil), allowed.Trusted...)
		facts.LocalServed = append([]OtherFolder(nil), allowed.LocalServed...)
		change(&facts)
		return facts
	}

	cases := []struct {
		name      string
		facts     ServeFacts
		code      string
		otherPath string
		otherName string
	}{
		{name: "allowed", facts: allowed},
		{
			name:  "allowed already trusted",
			facts: with(func(f *ServeFacts) { f.Trusted = []OtherFolder{{Relation: pathrel.Same, Path: site}} }),
		},
		{
			name:  "allowed unrelated served",
			facts: with(func(f *ServeFacts) { f.Served[0].Relation = pathrel.Unrelated }),
		},
		{
			name:  "not a folder: missing",
			facts: with(func(f *ServeFacts) { f.Exists = false }),
			code:  RefuseNotAFolder,
		},
		{
			name:  "not a folder: file",
			facts: with(func(f *ServeFacts) { f.IsDir = false }),
			code:  RefuseNotAFolder,
		},
		{
			name:  "is home",
			facts: with(func(f *ServeFacts) { f.Home = pathrel.Same }),
			code:  RefuseIsHome,
		},
		{
			name:  "outside home",
			facts: with(func(f *ServeFacts) { f.Home = pathrel.Unrelated }),
			code:  RefuseOutsideHome,
		},
		{
			name:  "outside home beats name taken",
			facts: with(func(f *ServeFacts) { f.Home = pathrel.Contains; f.NameErr = config.ErrServedNameTaken }),
			code:  RefuseOutsideHome,
		},
		{
			name: "protected folder inside",
			facts: with(func(f *ServeFacts) {
				f.Protected = []OtherFolder{{Relation: pathrel.Inside, Path: configDir}}
			}),
			code:      RefuseProtectedFolder,
			otherPath: configDir,
		},
		{
			name: "overlaps served: same",
			facts: with(func(f *ServeFacts) {
				f.Served = []OtherFolder{{Relation: pathrel.Same, Path: served, Name: "served"}}
			}),
			code:      RefuseOverlapsServed,
			otherPath: served,
			otherName: "served",
		},
		{
			name: "overlaps served: inside",
			facts: with(func(f *ServeFacts) {
				f.Served = []OtherFolder{{Relation: pathrel.Inside, Path: served, Name: "served"}}
			}),
			code:      RefuseOverlapsServed,
			otherPath: served,
			otherName: "served",
		},
		{
			name: "overlaps served: contains",
			facts: with(func(f *ServeFacts) {
				f.Served = []OtherFolder{{Relation: pathrel.Contains, Path: served, Name: "served"}}
			}),
			code:      RefuseOverlapsServed,
			otherPath: served,
			otherName: "served",
		},
		{
			name: "served by local",
			facts: with(func(f *ServeFacts) {
				f.LocalServed = []OtherFolder{{Relation: pathrel.Contains, Path: "/home/u/Local"}}
			}),
			code:      RefuseServedByLocal,
			otherPath: "/home/u/Local",
		},
		{
			name: "bound to local sync",
			facts: with(func(f *ServeFacts) {
				f.SyncMarkers = []SyncMarker{{Folder: "/home/sam/notes", RootID: "abcdef0123456789"}}
			}),
			code: RefuseBoundToLocalSync,
		},
		{
			name: "nested trusted: asked folder inside a trusted folder",
			facts: with(func(f *ServeFacts) {
				f.Trusted = []OtherFolder{{Relation: pathrel.Inside, Path: trusted}}
			}),
			code:      RefuseNestedTrusted,
			otherPath: trusted,
		},
		{
			name: "nested trusted: trusted folder inside the asked folder",
			facts: with(func(f *ServeFacts) {
				f.Trusted = []OtherFolder{{Relation: pathrel.Contains, Path: trusted}}
			}),
			code:      RefuseNestedTrusted,
			otherPath: trusted,
		},
		{
			name:  "name invalid",
			facts: with(func(f *ServeFacts) { f.NameErr = config.ErrServedNameInvalid }),
			code:  RefuseNameInvalid,
		},
		{
			name:  "name taken",
			facts: with(func(f *ServeFacts) { f.NameErr = config.ErrServedNameTaken }),
			code:  RefuseNameTaken,
		},
		{
			name:  "name retired",
			facts: with(func(f *ServeFacts) { f.NameErr = config.ErrServedNameRetired }),
			code:  RefuseNameRetired,
		},
		{
			name:  "name other error",
			facts: with(func(f *ServeFacts) { f.NameErr = errors.New("no name") }),
			code:  RefuseNameInvalid,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			refusal := CheckServe(c.facts)
			if c.code == "" {
				if refusal != nil {
					t.Fatalf("CheckServe refused %+v, want nil", refusal)
				}
				return
			}
			if refusal == nil {
				t.Fatalf("CheckServe allowed the folder, want %q", c.code)
			}
			if refusal.Code != c.code {
				t.Fatalf("code = %q, want %q", refusal.Code, c.code)
			}
			if refusal.OtherPath != c.otherPath {
				t.Errorf("other path = %q, want %q", refusal.OtherPath, c.otherPath)
			}
			if refusal.OtherName != c.otherName {
				t.Errorf("other name = %q, want %q", refusal.OtherName, c.otherName)
			}
		})
	}
}

// TestCheckServeNamesTheFieldAtFault proves a serve refusal says which field a
// form should mark: the name when it is taken, the path when it is outside home.
func TestCheckServeNamesTheFieldAtFault(t *testing.T) {
	taken := CheckServe(ServeFacts{
		Path:    "/home/u/Sites/site",
		Exists:  true,
		IsDir:   true,
		Home:    pathrel.Inside,
		NameErr: config.ErrServedNameTaken,
	})
	if taken == nil || taken.Code != RefuseNameTaken || taken.Field != "name" {
		t.Fatalf("taken name = %+v, want code %q with field %q", taken, RefuseNameTaken, "name")
	}

	outside := CheckServe(ServeFacts{
		Path:   "/elsewhere/site",
		Exists: true,
		IsDir:  true,
		Home:   pathrel.Unrelated,
	})
	if outside == nil || outside.Code != RefuseOutsideHome || outside.Field != "path" {
		t.Fatalf("outside home = %+v, want code %q with field %q", outside, RefuseOutsideHome, "path")
	}
}
