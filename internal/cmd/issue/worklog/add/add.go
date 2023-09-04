package add

import (
	"fmt"
	"time"

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
$ jira issue worklog add ISSUE-1 60m "My worklog" "2022-02-02" "13:35"

# Multi-line worklog
$ jira issue worklog add ISSUE-1 2h $'Supports\n\nNew line'

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
		Use:     "add [ISSUE-KEY] [TIME_SPENT] [WORKLOG_BODY] [STARTED_DATE] [STARTED_TIME]",
		Short:   "Add a comment to an issue",
		Long:    helpText,
		Example: examples,
		Annotations: map[string]string{
			"help:args": "ISSUE-KEY\tIssue key of the source issue, eg: ISSUE-1\n" +
				"TIME_SPENT\tTime spent in format '30m' or '4h 20m', etc.\n" +
				"WORKLOG_BODY\tBody of the worklog you want to add\n" +
				"STARTED_DATE\tDate in format '2022-05-15'\n" +
				"STARTED_TIME\tTime in format '15:55'",
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

	// cmdutil.ExitIfError(ac.setIssueKey())

	qs := ac.getQuestions()
	if len(qs) > 0 {
		ans := struct{ IssueKey, Comment, StartedDate, StartedTime, TimeSpent string }{}
		err := survey.Ask(qs, &ans)
		cmdutil.ExitIfError(err)

		if params.issueKey == "" {
			params.issueKey = ans.IssueKey
		}
		if params.comment == "" {
			params.comment = ans.Comment
		}
		if params.startedDate == "" {
			params.startedDate = ans.StartedDate
		}
		if params.startedTime == "" {
			params.startedTime = ans.StartedTime
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

		return client.AddIssueWorklog(ac.params.issueKey, ac.params.comment, ac.params.startedDate+"T"+params.startedTime+":00.000+0100", ac.params.timeSpent)
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
	issueKey    string
	comment     string
	startedDate string
	startedTime string
	timeSpent   string
	template    string
	noInput     bool
	debug       bool
}

func parseArgsAndFlags(args []string, flags query.FlagParser) *addParams {
	var issueKey, timeSpent, comment, startedDate, startedTime string

	nargs := len(args)
	if nargs >= 1 {
		issueKey = cmdutil.GetJiraIssueKey(viper.GetString("project.key"), args[0])
	}

	if nargs >= 2 {
		timeSpent = args[1]
	}

	if nargs >= 3 {
		comment = args[2]
	}

	if nargs >= 4 {
		startedDate = args[3]
	}

	if nargs >= 5 {
		startedTime = args[4]
	}

	debug, err := flags.GetBool("debug")
	cmdutil.ExitIfError(err)

	template, err := flags.GetString("template")
	cmdutil.ExitIfError(err)

	noInput, err := flags.GetBool("no-input")
	cmdutil.ExitIfError(err)

	return &addParams{
		issueKey:    issueKey,
		comment:     comment,
		startedDate: startedDate,
		startedTime: startedTime,
		timeSpent:   timeSpent,
		template:    template,
		noInput:     noInput,
		debug:       debug,
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

	currentTime := time.Now()

	var defaultBody string
	defaultTimeSpent := "60m"
	defaultComment := "Implementation"
	defaultDate := currentTime.Format("2006-01-02")
	defaultTime := currentTime.Format("15:04")

	if ac.params.timeSpent == "" {
		qs = append(qs, &survey.Question{
			Name:   "timeSpent",
			Prompt: &survey.Input{Message: "Worklog time spent", Default: defaultTimeSpent},
		})
	}

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
					Default:       defaultComment,
					HideDefault:   false,
					AppendDefault: true,
				},
				BlankAllowed: false,
			},
		})
	}

	if ac.params.startedDate == "" {
		qs = append(qs, &survey.Question{
			Name:   "startedDate",
			Prompt: &survey.Input{Message: "Worklog started date (YYYY-MM-DD)", Default: defaultDate},
		})
	}

	if ac.params.startedTime == "" {
		qs = append(qs, &survey.Question{
			Name:   "startedTime",
			Prompt: &survey.Input{Message: "Worklog started time (hh:mm)", Default: defaultTime},
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
