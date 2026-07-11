package agent

import (
	"context"

	"github.com/leftmike/gait/config"
	"github.com/leftmike/gait/mcpclient"
	"github.com/leftmike/gait/model"
	"github.com/leftmike/gait/skill"
	"github.com/leftmike/gait/tool"
)

type Agent struct {
	Client model.Client
	Model  model.Model
	Tools  map[string]tool.Tool
	Skills []*skill.Skill
	clnts  []*mcpclient.Client // XXX: rename field
}

func (ag *Agent) Add(tl tool.Tool) {
	if ag.Tools == nil {
		ag.Tools = map[string]tool.Tool{}
	}

	ag.Tools[tl.Name] = tl
}

func (ag *Agent) AddServer(ctx context.Context, svrCfg config.MCPServer, verbose bool) error {
	clnt, err := mcpclient.NewClient(ctx, svrCfg, verbose)
	if err != nil {
		return err
	}
	clnt.AddTools(ag.Tools)
	ag.clnts = append(ag.clnts, clnt)
	return nil
}

func (ag *Agent) AddSkill(dir string) error {
	skills, err := skill.ReadDir(dir)
	if err != nil {
		return err
	}

	ag.Add(tool.ReadFile)
	// XXX: ag.Add(tool.ListFiles()) -- maybe GlobFiles instead?

	for _, sk := range skills {
		// XXX: ag.fs.AddTree(sk.Dir, false)
		ag.Skills = append(ag.Skills, sk)
	}

	return nil
}

func (ag *Agent) SystemPrompt(st model.State) {
	if len(ag.Skills) > 0 {
		st.SystemPrompt(skill.SystemPrompt(ag.Skills))
	}
}
