package server

import "testing"

func TestBuildTreeDirsBeforeFilesAlphabetical(t *testing.T) {
	root := buildTree([]string{"z.txt", "a.txt", "sub/m.txt", "sub/a.txt"})

	if len(root.Children) != 3 {
		t.Fatalf("len(root.Children) = %d, want 3", len(root.Children))
	}
	// dirs first, then files alphabetically.
	want := []string{"sub", "a.txt", "z.txt"}
	for i, w := range want {
		if root.Children[i].Name != w {
			t.Errorf("root.Children[%d].Name = %q, want %q", i, root.Children[i].Name, w)
		}
	}
	if !root.Children[0].IsDir {
		t.Errorf("root.Children[0] (%q) should be a directory", root.Children[0].Name)
	}

	sub := root.Children[0]
	if len(sub.Children) != 2 {
		t.Fatalf("len(sub.Children) = %d, want 2", len(sub.Children))
	}
	if sub.Children[0].Name != "a.txt" || sub.Children[1].Name != "m.txt" {
		t.Errorf("sub.Children = [%q, %q], want [a.txt, m.txt]", sub.Children[0].Name, sub.Children[1].Name)
	}
	if sub.Children[0].Path != "sub/a.txt" {
		t.Errorf("sub.Children[0].Path = %q, want sub/a.txt", sub.Children[0].Path)
	}
}

func TestBuildTreeEmpty(t *testing.T) {
	root := buildTree(nil)
	if len(root.Children) != 0 {
		t.Errorf("root.Children = %v, want empty", root.Children)
	}
}

func TestBuildTreeSharedDirectoryPrefix(t *testing.T) {
	root := buildTree([]string{"sub/a.txt", "sub/nested/b.txt"})
	if len(root.Children) != 1 {
		t.Fatalf("len(root.Children) = %d, want 1 (single shared 'sub' dir)", len(root.Children))
	}
	sub := root.Children[0]
	if len(sub.Children) != 2 {
		t.Fatalf("len(sub.Children) = %d, want 2", len(sub.Children))
	}
}
