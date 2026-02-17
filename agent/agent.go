package agent

import (
	"context"

	"github.com/leftmike/gait/config"
	"github.com/leftmike/gait/filesys"
	"github.com/leftmike/gait/mcpclient"
	"github.com/leftmike/gait/model"
	"github.com/leftmike/gait/skill"
)

type Agent struct {
	mdl    model.Model
	tools  map[string]model.Tool
	skills []*skill.Skill
	FS     *filesys.ForestFS // XXX: should be fs
	clnts  []*mcpclient.Client
}

func NewAgent(mdl model.Model) *Agent {
	return &Agent{
		mdl:   mdl,
		tools: map[string]model.Tool{},
		FS:    filesys.NewForestFS(),
	}
}

func (ag *Agent) Close() {
	ag.FS.Close()
}

func (ag *Agent) AddTool(name, desc string, fn model.ToolFunc, scm model.ToolSchema) {
	ag.tools[name] = model.Tool{
		Name:        name,
		Description: desc,
		Func:        fn,
		Schema:      scm,
	}
}

func (ag *Agent) AddServer(ctx context.Context, svrCfg config.MCPServer, verbose bool) error {
	clnt, err := mcpclient.NewClient(ctx, svrCfg, verbose)
	if err != nil {
		return err
	}
	clnt.AddTools(ag.tools)
	ag.clnts = append(ag.clnts, clnt)
	return nil
}

func (ag *Agent) AddSkill(dir string) error {
	dirSkills, err := skill.ReadDir(dir)
	if err != nil {
		return err
	}

	// XXX: add dirSkills to fs

	ag.skills = append(ag.skills, dirSkills...)
	return nil
}

func (ag *Agent) Skills() []*skill.Skill {
	return ag.skills
}

func (ag *Agent) SystemPrompt(st model.State) {
	if len(ag.skills) > 0 {
		st.SystemPrompt(skill.SystemPrompt(ag.skills))
	}
}

func (ag *Agent) Generate(ctx context.Context, st model.State, opts *model.Options) error {
	return ag.mdl.Generate(ctx, st, ag.tools, opts)
}
