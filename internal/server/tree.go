package server

import (
	"sort"
	"strings"
)

// treeNode is one entry in the file-browser sidebar tree, built from the
// flat, sorted path list connector.ListFiles returns.
type treeNode struct {
	Name     string
	Path     string // set only on file (non-dir) nodes
	IsDir    bool
	Children []*treeNode
}

// buildTree turns a flat list of slash-separated relative paths into a
// nested tree, directories before files and alphabetical within each
// group.
func buildTree(paths []string) *treeNode {
	root := &treeNode{IsDir: true}
	for _, p := range paths {
		insertPath(root, p)
	}
	sortTree(root)
	return root
}

func insertPath(root *treeNode, path string) {
	parts := strings.Split(path, "/")
	cur := root
	for i, part := range parts {
		isFile := i == len(parts)-1

		var child *treeNode
		for _, c := range cur.Children {
			if c.Name == part && c.IsDir == !isFile {
				child = c
				break
			}
		}
		if child == nil {
			child = &treeNode{Name: part, IsDir: !isFile}
			if isFile {
				child.Path = path
			}
			cur.Children = append(cur.Children, child)
		}
		cur = child
	}
}

func sortTree(n *treeNode) {
	sort.Slice(n.Children, func(i, j int) bool {
		a, b := n.Children[i], n.Children[j]
		if a.IsDir != b.IsDir {
			return a.IsDir
		}
		return a.Name < b.Name
	})
	for _, c := range n.Children {
		if c.IsDir {
			sortTree(c)
		}
	}
}
