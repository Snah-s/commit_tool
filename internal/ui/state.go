package ui

import (
	"github.com/Snah-s/commit_tool/internal/catalog"
	"github.com/Snah-s/commit_tool/internal/commit"
)

type Draft = commit.Draft

func reconcile(d *Draft, c catalog.Catalog) {
	if d.Mode == "standard" || (d.Mode == "hybrid" && !c.Compatible(d.Type, d.Emoji)) {
		d.Emoji = ""
	}
}
