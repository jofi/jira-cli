package add

import (
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/ankitpokhrel/jira-cli/api"
	"github.com/ankitpokhrel/jira-cli/internal/cmdcommon"
	"github.com/ankitpokhrel/jira-cli/internal/cmdutil"
	"github.com/ankitpokhrel/jira-cli/internal/query"
	"github.com/ankitpokhrel/jira-cli/pkg/jira"
	"github.com/ankitpokhrel/jira-cli/pkg/surveyext"
)

const (
	helpText = `Add adds worklog to an issue.`
	examples = `$ jira issue worklog add

# Pass required parameters to skip prompt 
$ jira issue comment add ISSUE-1 "My worklog"

# Multi-line worklog
$ jira issue worklog add ISSUE-1 $'Supports\n\nNew line'

# Load worklog body from a template file
$ jira issue worklog add ISSUE-1 --template /path/to/template.tmpl

# Get worklog body from standard input
$ jira issue worklog add ISSUE-1 --template -

# Or, use pipe to read input directly from standard input
$ echo "Worklog from stdin" | jira issue worklog add ISSUE-1

# Positional argument takes precedence over the template flag
# The example below will add "worklog from arg" as a worklog
$ jira issue comment add ISSUE-1 "worklog from arg" --template /path/to/template.tmpl`
)

// NewCmdCommentAdd is a comment add command.
func NewCmdCWorklogAdd() *cobra.Command {
	cmd := cobra.Command{
		Use:     "add ISSUE-KEY [COMMENT_BODY]",
		Short:   "Add a comment to an issue",
		Long:    helpText,
		Example: examples,
		Annotations: map[string]string{
			"help:args": "ISSUE-KEY\tIssue key of the source issue, eg: ISSUE-1\n" +
				"WORKLOG_BODY\tBody of the worklog you want to add",
		},
		Run: add,
	}

	cmd.Flags().Bool("web", false, "Open issue in web browser after adding worklog")
	cmd.Flags().StringP("template", "T", "", "Path to a file to read worklog body from")
	cmd.Flags().Bool("no-input", false, "Disable prompt for non-required fields")

	return &cmd
}

func add(cmd *cobra.Command, args []string) {
	params := parseArgsAndFlags(args, cmd.Flags())
	client := api.Client(jira.Config{Debug: params.debug})
	ac := addCmd{
		client:    client,
		linkTypes: nil,
		params:    params,
	}

	if ac.isNonInteractive() {
		ac.params.noInput = true

		if ac.isMandatoryParamsMissing() {
			cmdutil.Failed("`ISSUE-KEY` is mandatory when using a non-interactive mode")
		}
	}

	cmdutil.ExitIfError(ac.setIssueKey())

	qs := ac.getQuestions()
	if len(qs) > 0 {
		ans := struct{ IssueKey, Comment, Started, TimeSpent string }{}
		err := survey.Ask(qs, &ans)
		cmdutil.ExitIfError(err)

		if params.issueKey == "" {
			params.issueKey = ans.IssueKey
		}
		if params.comment == "" {
			params.comment = ans.Comment
		}
		if params.started == "" {
			params.started = ans.Started
		}
		if params.timeSpent == "" {
			params.timeSpent = ans.TimeSpent
		}
	}

	if !params.noInput {
		answer := struct{ Action string }{}
		err := survey.Ask([]*survey.Question{ac.getNextAction()}, &answer)
		cmdutil.ExitIfError(err)

		if answer.Action == cmdcommon.ActionCancel {
			cmdutil.Failed("Action aborted")
		}
	}

	err := func() error {
		s := cmdutil.Info("Adding worklog")
		defer s.Stop()

		return client.AddIssueWorklog(ac.params.issueKey, ac.params.comment, ac.params.started, ac.params.timeSpent)
	}()
	cmdutil.ExitIfError(err)

	server := viper.GetString("server")

	cmdutil.Success("Worklog added to issue \"%s\"", ac.params.issueKey)
	fmt.Printf("%s/browse/%s\n", server, ac.params.issueKey)

	if web, _ := cmd.Flags().GetBool("web"); web {
		err := cmdutil.Navigate(server, ac.params.issueKey)
		cmdutil.ExitIfError(err)
	}
}

type addParams struct {
	issueKey  string
	comment   string
	started   string
	timeSpent string
	template  string
	noInput   bool
	debug     bool
}

func parseArgsAndFlags(args []string, flags query.FlagParser) *addParams {
	var issueKey, comment, started, timeSpent string

	nargs := len(args)
	if nargs >= 1 {
		issueKey = cmdutil.GetJiraIssueKey(viper.GetString("project.key"), args[0])
	}
	if nargs >= 2 {
		comment = args[1]
	}

	if nargs >= 3 {
		started = args[2]
	}

	if nargs >= 4 {
		timeSpent = args[3]
	}

	debug, err := flags.GetBool("debug")
	cmdutil.ExitIfError(err)

	template, err := flags.GetString("template")
	cmdutil.ExitIfError(err)

	noInput, err := flags.GetBool("no-input")
	cmdutil.ExitIfError(err)

	return &addParams{
		issueKey:  issueKey,
		comment:   comment,
		started:   started,
		timeSpent: timeSpent,
		template:  template,
		noInput:   noInput,
		debug:     debug,
	}
}

type addCmd struct {
	client    *jira.Client
	linkTypes []*jira.IssueLinkType
	params    *addParams
}

func (ac *addCmd) setIssueKey() error {
	if ac.params.issueKey != "" {
		return nil
	}

	var ans string

	qs := &survey.Question{
		Name:     "issueKey",
		Prompt:   &survey.Input{Message: "Issue key"},
		Validate: survey.Required,
	}
	if err := survey.Ask([]*survey.Question{qs}, &ans); err != nil {
		return err
	}
	ac.params.issueKey = cmdutil.GetJiraIssueKey(viper.GetString("project.key"), ans)

	return nil
}

func (ac *addCmd) getQuestions() []*survey.Question {
	var qs []*survey.Question

	if ac.params.issueKey == "" {
		qs = append(qs, &survey.Question{
			Name:     "issueKey",
			Prompt:   &survey.Input{Message: "Issue key"},
			Validate: survey.Required,
		})
	}

	var defaultBody string

	if ac.params.template != "" || cmdutil.StdinHasData() {
		b, err := cmdutil.ReadFile(ac.params.template)
		if err != nil {
			cmdutil.Failed("Error: %s", err)
		}
		defaultBody = string(b)
	}

	if ac.params.noInput && ac.params.comment == "" {
		ac.params.comment = defaultBody
		return qs
	}

	if ac.params.comment == "" {
		qs = append(qs, &survey.Question{
			Name: "comment",
			Prompt: &surveyext.JiraEditor{
				Editor: &survey.Editor{
					Message:       "Worklog comment",
					Default:       defaultBody,
					HideDefault:   true,
					AppendDefault: true,
				},
				BlankAllowed: false,
			},
		})
	}

	if ac.params.started == "" {
		qs = append(qs, &survey.Question{
			Name: "started",
			Prompt: &surveyext.JiraEditor{
				Editor: &survey.Editor{
					Message:       "Worklog started",
					Default:       defaultBody,
					HideDefault:   true,
					AppendDefault: true,
				},
				BlankAllowed: false,
			},
		})
	}

	if ac.params.timeSpent == "" {
		qs = append(qs, &survey.Question{
			Name: "timeSpent",
			Prompt: &surveyext.JiraEditor{
				Editor: &survey.Editor{
					Message:       "Worklog time spent",
					Default:       defaultBody,
					HideDefault:   true,
					AppendDefault: true,
				},
				BlankAllowed: false,
			},
		})
	}

	return qs
}

func (ac *addCmd) getNextAction() *survey.Question {
	return &survey.Question{
		Name: "action",
		Prompt: &survey.Select{
			Message: "What's next?",
			Options: []string{
				cmdcommon.ActionSubmit,
				cmdcommon.ActionCancel,
			},
		},
		Validate: survey.Required,
	}
}

func (ac *addCmd) isNonInteractive() bool {
	return cmdutil.StdinHasData() || ac.params.template == "-"
}

func (ac *addCmd) isMandatoryParamsMissing() bool {
	return ac.params.issueKey == ""
}
