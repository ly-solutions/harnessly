package main

import (
	"context"

	"github.com/ly-solutions/harnessly/internal/jsvalue"
	"github.com/ly-solutions/harnessly/internal/services"
	"github.com/ly-solutions/harnessly/internal/wire"
)

// madeTaskOf reads a tracker task the way the task card does (one request per task, kept a minute) and tells of it
// what the made block shows.
func madeTaskOf(s *services.Service) madeTaskReader {
	if s == nil {
		return nil
	}
	return func(ctx context.Context, key string) (string, wire.MadeTaskInfo, bool) {
		task, _ := s.Task(ctx, key)
		if task == nil {
			return "", wire.MadeTaskInfo{}, false
		}
		info := wire.MadeTaskInfo{
			Key:      task.String("key"),
			Type:     task.String("type"),
			State:    task.String("state"),
			Resolved: flag(task, "resolved"),
			Children: []wire.MadeTaskRef{},
		}
		if p := task.Object("parent"); p != nil {
			info.Parent = &wire.MadeTaskRef{Key: p.String("key"), Title: p.String("title")}
		}
		for _, c := range task.Array("children") {
			o, ok := c.(*jsvalue.Object)
			if !ok || len(info.Children) == wire.MadeTaskChildrenMax {
				continue
			}
			info.Children = append(info.Children, wire.MadeTaskRef{
				Key: o.String("key"), Title: o.String("title"), State: o.String("state"), Resolved: flag(o, "resolved"),
			})
		}
		return task.String("title"), info, info.Key != ""
	}
}

func flag(o *jsvalue.Object, k string) bool {
	v, _ := o.Get(k)
	b, _ := v.(bool)
	return b
}
