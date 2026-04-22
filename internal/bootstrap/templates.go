package bootstrap

import (
	"fmt"
	"path/filepath"
	"time"

	"obsidian-harness/internal/model"
)

type Template struct {
	Ref     model.DocumentRef `json:"ref"`
	Content string            `json:"content"`
}

func DefaultManagedTemplates(now time.Time) []Template {
	weekStart := now.AddDate(0, 0, -int(now.Weekday()-1))
	if now.Weekday() == time.Sunday {
		weekStart = now.AddDate(0, 0, -6)
	}
	weekEnd := weekStart.AddDate(0, 0, 6)
	weekName := fmt.Sprintf(
		"Week1逐日执行表-%s至%s.md",
		weekStart.Format("2006-01-02"),
		weekEnd.Format("2006-01-02"),
	)

	return []Template{
		{
			Ref: model.DocumentRef{
				Path:  filepath.Join("00-系统", "系统说明.md"),
				Class: model.DocClassSystemDoc,
			},
			Content: "# 系统说明\n\n- 目标: 说明本 Obsidian 工作区的治理边界和协作方式。\n- Managed Mode: 已启用。\n- 三件套: 系统说明 / 文档进度总表 / 人物画像。\n",
		},
		{
			Ref: model.DocumentRef{
				Path:  filepath.Join("0-排期", "00-系统", "文档进度总表.md"),
				Class: model.DocClassProgressIndex,
			},
			Content: "# 文档进度总表\n\n| 文档 | 类型 | 状态 | 最近更新 |\n| --- | --- | --- | --- |\n| 系统说明 | 系统文档 | 在管 | |\n| 人物画像 | 画像 | 在管 | |\n",
		},
		{
			Ref: model.DocumentRef{
				Path:  filepath.Join("03-画像", "人物画像.md"),
				Class: model.DocClassPersona,
			},
			Content: "# 人物画像\n\n## Identity / Stable Profile\n\n- 姓名:\n- 目标角色:\n\n## Current State\n\n- 当前重点:\n- 当前阻塞:\n\n## Weaknesses\n\n- 待补充\n\n## Evidence Log\n\n- 待补充\n\n## Freeform Notes\n\n- 待补充\n",
		},
		{
			Ref: model.DocumentRef{
				Path:  filepath.Join("0-排期", "04-执行", weekName),
				Class: model.DocClassPlanWeek,
			},
			Content: fmt.Sprintf("# Week1逐日执行表\n\n- 周期: %s 至 %s\n- [ ] 今日重点 1\n- [ ] 今日重点 2\n", weekStart.Format("2006-01-02"), weekEnd.Format("2006-01-02")),
		},
	}
}
