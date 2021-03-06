/*
Copyright 2018 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	"archive/tar"
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/GoogleContainerTools/kaniko/testutil"
)

func Test_fileSystemWhitelist(t *testing.T) {
	testDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("Error creating tempdir: %s", err)
	}
	fileContents := `
	228 122 0:90 / / rw,relatime - aufs none rw,si=f8e2406af90782bc,dio,dirperm1
	229 228 0:98 / /proc rw,nosuid,nodev,noexec,relatime - proc proc rw
	230 228 0:99 / /dev rw,nosuid - tmpfs tmpfs rw,size=65536k,mode=755
	231 230 0:100 / /dev/pts rw,nosuid,noexec,relatime - devpts devpts rw,gid=5,mode=620,ptmxmode=666
	232 228 0:101 / /sys ro,nosuid,nodev,noexec,relatime - sysfs sysfs ro`

	path := filepath.Join(testDir, "mountinfo")
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		t.Fatalf("Error creating tempdir: %s", err)
	}
	if err := ioutil.WriteFile(path, []byte(fileContents), 0644); err != nil {
		t.Fatalf("Error writing file contents to %s: %s", path, err)
	}

	actualWhitelist, err := fileSystemWhitelist(path)
	expectedWhitelist := []string{"/kaniko", "/proc", "/dev", "/dev/pts", "/sys", "/var/run"}
	sort.Strings(actualWhitelist)
	sort.Strings(expectedWhitelist)
	testutil.CheckErrorAndDeepEqual(t, false, err, expectedWhitelist, actualWhitelist)
}

var tests = []struct {
	files         map[string]string
	directory     string
	expectedFiles []string
}{
	{
		files: map[string]string{
			"/workspace/foo/a": "baz1",
			"/workspace/foo/b": "baz2",
			"/kaniko/file":     "file",
		},
		directory: "/workspace/foo/",
		expectedFiles: []string{
			"workspace/foo/a",
			"workspace/foo/b",
			"workspace/foo",
		},
	},
	{
		files: map[string]string{
			"/workspace/foo/a": "baz1",
		},
		directory: "/workspace/foo/a",
		expectedFiles: []string{
			"workspace/foo/a",
		},
	},
	{
		files: map[string]string{
			"/workspace/foo/a": "baz1",
			"/workspace/foo/b": "baz2",
			"/workspace/baz":   "hey",
			"/kaniko/file":     "file",
		},
		directory: "/workspace",
		expectedFiles: []string{
			"workspace/foo/a",
			"workspace/foo/b",
			"workspace/baz",
			"workspace",
			"workspace/foo",
		},
	},
	{
		files: map[string]string{
			"/workspace/foo/a": "baz1",
			"/workspace/foo/b": "baz2",
			"/kaniko/file":     "file",
		},
		directory: "",
		expectedFiles: []string{
			"workspace/foo/a",
			"workspace/foo/b",
			"kaniko/file",
			"workspace",
			"workspace/foo",
			"kaniko",
			".",
		},
	},
}

func Test_RelativeFiles(t *testing.T) {
	for _, test := range tests {
		testDir, err := ioutil.TempDir("", "")
		if err != nil {
			t.Fatalf("err setting up temp dir: %v", err)
		}
		defer os.RemoveAll(testDir)
		if err := testutil.SetupFiles(testDir, test.files); err != nil {
			t.Fatalf("err setting up files: %v", err)
		}
		actualFiles, err := RelativeFiles(test.directory, testDir)
		sort.Strings(actualFiles)
		sort.Strings(test.expectedFiles)
		testutil.CheckErrorAndDeepEqual(t, false, err, test.expectedFiles, actualFiles)
	}
}

func Test_ParentDirectories(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected []string
	}{
		{
			name: "regular path",
			path: "/path/to/dir",
			expected: []string{
				"/path",
				"/path/to",
			},
		},
		{
			name:     "current directory",
			path:     ".",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := ParentDirectories(tt.path)
			testutil.CheckErrorAndDeepEqual(t, false, nil, tt.expected, actual)
		})
	}
}

func Test_checkWhiteouts(t *testing.T) {
	type args struct {
		path      string
		whiteouts map[string]struct{}
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "file whited out",
			args: args{
				path:      "/foo",
				whiteouts: map[string]struct{}{"/foo": {}},
			},
			want: true,
		},
		{
			name: "directory whited out",
			args: args{
				path:      "/foo/bar",
				whiteouts: map[string]struct{}{"/foo": {}},
			},
			want: true,
		},
		{
			name: "grandparent whited out",
			args: args{
				path:      "/foo/bar/baz",
				whiteouts: map[string]struct{}{"/foo": {}},
			},
			want: true,
		},
		{
			name: "sibling whited out",
			args: args{
				path:      "/foo/bar/baz",
				whiteouts: map[string]struct{}{"/foo/bat": {}},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkWhiteouts(tt.args.path, tt.args.whiteouts); got != tt.want {
				t.Errorf("checkWhiteouts() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_checkWhitelist(t *testing.T) {
	type args struct {
		path      string
		whitelist []string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "file whitelisted",
			args: args{
				path:      "/foo",
				whitelist: []string{"/foo"},
			},
			want: true,
		},
		{
			name: "directory whitelisted",
			args: args{
				path:      "/foo/bar",
				whitelist: []string{"/foo"},
			},
			want: true,
		},
		{
			name: "grandparent whitelisted",
			args: args{
				path:      "/foo/bar/baz",
				whitelist: []string{"/foo"},
			},
			want: true,
		},
		{
			name: "sibling whitelisted",
			args: args{
				path:      "/foo/bar/baz",
				whitelist: []string{"/foo/bat"},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkWhitelist(tt.args.path, tt.args.whitelist); got != tt.want {
				t.Errorf("checkWhitelist() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasFilepathPrefix(t *testing.T) {
	type args struct {
		path   string
		prefix string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "parent",
			args: args{
				path:   "/foo/bar",
				prefix: "/foo",
			},
			want: true,
		},
		{
			name: "nested parent",
			args: args{
				path:   "/foo/bar/baz",
				prefix: "/foo/bar",
			},
			want: true,
		},
		{
			name: "sibling",
			args: args{
				path:   "/foo/bar",
				prefix: "/bar",
			},
			want: false,
		},
		{
			name: "nested sibling",
			args: args{
				path:   "/foo/bar/baz",
				prefix: "/foo/bar",
			},
			want: true,
		},
		{
			name: "name prefix",
			args: args{
				path:   "/foo2/bar",
				prefix: "/foo",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasFilepathPrefix(tt.args.path, tt.args.prefix); got != tt.want {
				t.Errorf("HasFilepathPrefix() = %v, want %v", got, tt.want)
			}
		})
	}
}

type checker func(root string, t *testing.T)

func fileExists(p string) checker {
	return func(root string, t *testing.T) {
		_, err := os.Stat(filepath.Join(root, p))
		if err != nil {
			t.Fatalf("File does not exist")
		}
	}
}

func fileMatches(p string, c []byte) checker {
	return func(root string, t *testing.T) {
		actual, err := ioutil.ReadFile(filepath.Join(root, p))
		if err != nil {
			t.Fatalf("error reading file: %s", p)
		}
		if !reflect.DeepEqual(actual, c) {
			t.Errorf("file contents do not match. %v!=%v", actual, c)
		}
	}
}

func permissionsMatch(p string, perms os.FileMode) checker {
	return func(root string, t *testing.T) {
		fi, err := os.Stat(filepath.Join(root, p))
		if err != nil {
			t.Fatalf("error statting file %s", p)
		}
		if fi.Mode() != perms {
			t.Errorf("Permissions do not match. %s != %s", fi.Mode(), perms)
		}
	}
}

func linkPointsTo(src, dst string) checker {
	return func(root string, t *testing.T) {
		link := filepath.Join(root, src)
		got, err := os.Readlink(link)
		if err != nil {
			t.Fatalf("error reading link %s: %s", link, err)
		}
		if got != dst {
			t.Errorf("link destination does not match: %s != %s", got, dst)
		}
	}
}

func fileHeader(name string, contents string, mode int64) *tar.Header {
	return &tar.Header{
		Name:     name,
		Size:     int64(len(contents)),
		Mode:     mode,
		Typeflag: tar.TypeReg,
	}
}

func linkHeader(name, linkname string) *tar.Header {
	return &tar.Header{
		Name:     name,
		Size:     0,
		Typeflag: tar.TypeSymlink,
		Linkname: linkname,
	}
}

func hardlinkHeader(name, linkname string) *tar.Header {
	return &tar.Header{
		Name:     name,
		Size:     0,
		Typeflag: tar.TypeLink,
		Linkname: linkname,
	}
}

func dirHeader(name string, mode int64) *tar.Header {
	return &tar.Header{
		Name:     name,
		Size:     0,
		Typeflag: tar.TypeDir,
		Mode:     mode,
	}
}

func TestExtractFile(t *testing.T) {
	type tc struct {
		name     string
		hdrs     []*tar.Header
		contents []byte
		checkers []checker
	}

	tcs := []tc{
		{
			name:     "normal file",
			contents: []byte("helloworld"),
			hdrs:     []*tar.Header{fileHeader("./bar", "helloworld", 0644)},
			checkers: []checker{
				fileExists("/bar"),
				fileMatches("/bar", []byte("helloworld")),
				permissionsMatch("/bar", 0644),
			},
		},
		{
			name:     "normal file, directory does not exist",
			contents: []byte("helloworld"),
			hdrs:     []*tar.Header{fileHeader("./foo/bar", "helloworld", 0644)},
			checkers: []checker{
				fileExists("/foo/bar"),
				fileMatches("/foo/bar", []byte("helloworld")),
				permissionsMatch("/foo/bar", 0644),
				permissionsMatch("/foo", 0755|os.ModeDir),
			},
		},
		{
			name:     "normal file, directory is created after",
			contents: []byte("helloworld"),
			hdrs: []*tar.Header{
				fileHeader("./foo/bar", "helloworld", 0644),
				dirHeader("./foo", 0722),
			},
			checkers: []checker{
				fileExists("/foo/bar"),
				fileMatches("/foo/bar", []byte("helloworld")),
				permissionsMatch("/foo/bar", 0644),
				permissionsMatch("/foo", 0722|os.ModeDir),
			},
		},
		{
			name: "symlink",
			hdrs: []*tar.Header{linkHeader("./bar", "bar/bat")},
			checkers: []checker{
				linkPointsTo("/bar", "bar/bat"),
			},
		},
		{
			name: "symlink relative path",
			hdrs: []*tar.Header{linkHeader("./bar", "./foo/bar/baz")},
			checkers: []checker{
				linkPointsTo("/bar", "./foo/bar/baz"),
			},
		},
		{
			name: "symlink parent does not exist",
			hdrs: []*tar.Header{linkHeader("./foo/bar/baz", "../../bat")},
			checkers: []checker{
				linkPointsTo("/foo/bar/baz", "../../bat"),
			},
		},
		{
			name: "symlink parent does not exist",
			hdrs: []*tar.Header{linkHeader("./foo/bar/baz", "../../bat")},
			checkers: []checker{
				linkPointsTo("/foo/bar/baz", "../../bat"),
				permissionsMatch("/foo", 0755|os.ModeDir),
				permissionsMatch("/foo/bar", 0755|os.ModeDir),
			},
		},
		{
			name: "hardlink",
			hdrs: []*tar.Header{
				fileHeader("/bin/gzip", "gzip-binary", 0751),
				hardlinkHeader("/bin/uncompress", "/bin/gzip"),
			},
			checkers: []checker{
				linkPointsTo("/bin/uncompress", "/bin/gzip"),
			},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			tc := tc
			t.Parallel()
			r, err := ioutil.TempDir("", "")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(r)
			for _, hdr := range tc.hdrs {
				if err := extractFile(r, hdr, bytes.NewReader(tc.contents)); err != nil {
					t.Fatal(err)
				}
			}
			for _, checker := range tc.checkers {
				checker(r, t)
			}
		})
	}
}
