package agent

import (
	"context"
	"encoding/json"
	"io/ioutil"

	"github.com/leftmike/gait/config"
	"github.com/leftmike/gait/mcpclient"
	"github.com/leftmike/gait/model"
	"github.com/leftmike/gait/skill"
)

type Agent struct {
	provider  string
	ModelName string
	apiKey    string
	mdl       model.Model
	tools     map[string]model.Tool
	skills    []*skill.Skill
	clnts     []*mcpclient.Client
}

func NewAgent(provider, modelName, apiKey string, mdl model.Model) *Agent {
	return &Agent{
		provider:  provider,
		ModelName: modelName,
		apiKey:    apiKey,
		mdl:       mdl,
		tools:     map[string]model.Tool{},
	}
}

func (ag *Agent) Model() model.Model {
	return ag.mdl
}

func (ag *Agent) Provider() string {
	return ag.provider
}

func (ag *Agent) APIKey() string {
	return ag.apiKey
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
	skills, err := skill.ReadDir(dir)
	if err != nil {
		return err
	}

	ag.AddReadFileTool()
	// XXX: ag.AddListFilesTool() -- maybe GlobFilesTool instead?

	for _, sk := range skills {
		// XXX: ag.fs.AddTree(sk.Dir, false)
		ag.skills = append(ag.skills, sk)
	}

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
	return ag.mdl.Generate(ctx, ag.ModelName, st, ag.tools, opts)
}

type readFileArgs struct {
	Path string `json:"path" gait:"the path of the file to read"`
}

func (ag *Agent) readFile(ctx context.Context, buf []byte) (string, error) {
	var args readFileArgs
	err := json.Unmarshal(buf, &args)
	if err != nil {
		return "", err
	}

	buf, err = ioutil.ReadFile(args.Path)
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

func (ag *Agent) AddReadFileTool() {
	if _, ok := ag.tools["read_file"]; !ok {
		ag.tools["read_file"] = model.Tool{
			Name:        "read_file",
			Description: "reads the contents of a file",
			Func:        ag.readFile,
			Schema:      model.MustToolSchema[readFileArgs](),
		}
	}
}
