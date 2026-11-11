//go:build managedtrial

package managedtrial

import (
	"os"
	"path/filepath"
	"strings"
)

// AD2: the one refusal this host is allowed. Local rebases a save made against
// an older page onto the stored one (page-rebase.js rebaseStaleSave); this host
// does not rebase, so it refuses that save instead, before anything is written.
// The predicate is exactly the case Local would rebase: the incoming content
// carries no inline page state, it carries a page revision, the stored file has
// its own exact blockeditor-state record at a different revision, and one of the 50
// newest versions of this document is at the incoming revision.

const staleVersionWindow = 50

// stalePageLineage reports whether a save of content would be a stale-blockeditor save.
// It only reads: nothing is written and nothing is cached.
func stalePageLineage(baseDir, name, content string) (bool, error) {
	if _, state := splitPage(content); state != nil {
		return false, nil
	}
	rev := pageRev(content)
	if rev == "" {
		return false, nil
	}

	data, err := os.ReadFile(upathJoin(baseDir, name))
	if err != nil {
		return false, nil
	}
	storedHTML := string(data)
	record, err := readPageState(baseDir, name, PageStateReadOptions{Rev: pageRev(storedHTML), Exact: true})
	if err != nil || record == nil || record.Rev == rev {
		return false, nil
	}

	versions, err := newestVersionFiles(baseDir, name)
	if err != nil {
		return false, nil
	}
	if len(versions) > staleVersionWindow {
		versions = versions[:staleVersionWindow]
	}
	for _, file := range versions {
		versionBytes, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		versionHTML := string(versionBytes)
		if strings.Contains(versionHTML, rev) && pageRev(versionHTML) == rev {
			return true, nil
		}
	}
	return false, nil
}

// rebaseVersions is what Local's rebaseStaleSave reads for one incoming page
// (page-rebase.js:25-58), sent with an admit so the service never reads the
// served folder: the newest staleVersionWindow version files whose page rev is
// the incoming one, when the incoming page carries a rev, no page state, and a
// rev other than the stored file's own record. Otherwise none. Unreadable
// history is none, as it is for the rebase.
func rebaseVersions(baseDir, name, incoming string) []orderedObject {
	found := []orderedObject{}
	if _, state := splitPage(incoming); state != nil {
		return found
	}
	rev := pageRev(incoming)
	if rev == "" {
		return found
	}
	data, err := os.ReadFile(upathJoin(baseDir, name))
	if err != nil {
		return found
	}
	record, err := readPageState(baseDir, name, PageStateReadOptions{Rev: pageRev(string(data)), Exact: true})
	if err != nil || record == nil || record.Rev == rev {
		return found
	}
	versions, err := newestVersionFiles(baseDir, name)
	if err != nil {
		return found
	}
	if len(versions) > staleVersionWindow {
		versions = versions[:staleVersionWindow]
	}
	for _, file := range versions {
		versionBytes, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		versionHTML := string(versionBytes)
		if strings.Contains(versionHTML, rev) && pageRev(versionHTML) == rev {
			found = append(found, orderedObject{
				{Key: "name", Value: filepath.Base(file)},
				{Key: "bytes", Value: controlBytesValue(versionBytes)},
			})
		}
	}
	return found
}

// newestVersionFiles is a document's version files, newest first, as the save
// path lists them: history is read only from inside the served folder, so a
// versions directory reached through a symlink yields nothing.
func newestVersionFiles(baseDir, name string) ([]string, error) {
	dir := upathJoin(baseDir, versionsDirName, backupSiteName(name))
	if err := AssertVersionsChain(baseDir, dir); err != nil {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil
	}
	found := make([]VersionEntry, 0, len(entries))
	for _, entry := range entries {
		filename := entry.Name()
		if !IsVersionName(filename) || !strings.HasSuffix(strings.ToLower(filename), ".html") {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		found = append(found, VersionEntry{
			Name:    filename,
			Full:    filepath.Join(dir, filename),
			MtimeMs: info.ModTime().UnixMilli(),
		})
	}
	sortNewestFirst(found)
	files := make([]string, len(found))
	for index, entry := range found {
		files[index] = entry.Full
	}
	return files, nil
}
