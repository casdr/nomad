// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package command

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/cli"
	"github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/helper/pointer"
	"github.com/posener/complete"
)

// Ensure JobScaleCommand satisfies the cli.Command interface.
var _ cli.Command = &JobScaleCommand{}

// JobScaleCommand implements cli.Command.
type JobScaleCommand struct {
	Meta
}

// Help satisfies the cli.Command Help function.
func (j *JobScaleCommand) Help() string {
	helpText := `
Usage: nomad job scale [options] <job> [<group>] <count>

  Perform a scaling action by altering the count within a job group.

  Upon successful job submission, this command will immediately
  enter an interactive monitor. This is useful to watch Nomad's
  internals make scheduling decisions and place the submitted work
  onto nodes. The monitor will end once job placement is done. It
  is safe to exit the monitor early using ctrl+c.

  When ACLs are enabled, this command requires a token with the
  'read-job-scaling' and either the 'scale-job' or 'submit-job' capabilities
  for the job's namespace. The 'list-jobs' capability is required to run the
  command with a job prefix instead of the exact job ID. The 'read-job'
  capability is required to monitor the resulting evaluation when -detach is
  not used.

General Options:

  ` + generalOptionsUsage(usageOptsDefault) + `

Scale Options:

  -check-index
    If set, the job is only scaled if the passed job modify index matches the
    server side version. Ignored if value of zero is passed. If a non-zero value
    is passed, it ensures that the job is being updated from a known state.

  -detach
    Return immediately instead of entering monitor mode. After job scaling,
    the evaluation ID will be printed to the screen, which can be used to
    examine the evaluation using the eval-status command.

  -verbose
    Display full information.
`
	return strings.TrimSpace(helpText)
}

// Synopsis satisfies the cli.Command Synopsis function.
func (j *JobScaleCommand) Synopsis() string {
	return "Change the count of a Nomad job group"
}

func (j *JobScaleCommand) AutocompleteFlags() complete.Flags {
	return mergeAutocompleteFlags(j.Meta.AutocompleteFlags(FlagSetClient),
		complete.Flags{
			"-check-index": complete.PredictNothing,
			"-detach":      complete.PredictNothing,
			"-verbose":     complete.PredictNothing,
		})
}

// Name returns the name of this command.
func (j *JobScaleCommand) Name() string { return "job scale" }

// Run satisfies the cli.Command Run function.
func (j *JobScaleCommand) Run(args []string) int {
	var detach, verbose bool
	var checkIndex uint64

	flags := j.Meta.FlagSet(j.Name(), FlagSetClient)
	flags.Usage = func() { j.Ui.Output(j.Help()) }
	flags.Uint64Var(&checkIndex, "check-index", 0, "")
	flags.BoolVar(&detach, "detach", false, "")
	flags.BoolVar(&verbose, "verbose", false, "")
	if err := flags.Parse(args); err != nil {
		return 1
	}

	var countString, groupString string
	args = flags.Args()

	// It is possible to specify either 2 or 3 arguments. Check and assign the
	// args so they can be validate later on.
	if numArgs := len(args); numArgs < 2 || numArgs > 3 {
		j.Ui.Error("Command requires at least two arguments and no more than three")
		return 1
	} else if numArgs == 3 {
		groupString = args[1]
		countString = args[2]
	} else {
		countString = args[1]
	}

	// Convert the count string arg to an int as required by the API.
	count, err := strconv.Atoi(countString)
	if err != nil {
		j.Ui.Error(fmt.Sprintf("Failed to convert count string to int: %s", err))
		return 1
	}

	// Get the HTTP client.
	client, err := j.Meta.Client()
	if err != nil {
		j.Ui.Error(fmt.Sprintf("Error initializing client: %s", err))
		return 1
	}

	// Check if the job exists
	jobIDPrefix := strings.TrimSpace(args[0])
	jobID, namespace, err := j.JobIDByPrefix(client, jobIDPrefix, nil)
	if err != nil {
		j.Ui.Error(err.Error())
		return 1
	}

	// Detail the job so we can perform addition checks before submitting the
	// scaling request.
	q := &api.QueryOptions{Namespace: namespace}
	jobScaleStatusResp, _, err := client.Jobs().ScaleStatus(jobID, q)
	if err != nil {
		j.Ui.Error(fmt.Sprintf("Error querying job: %v", err))
		return 1
	}

	if err := j.performGroupCheck(jobScaleStatusResp.TaskGroups, &groupString); err != nil {
		j.Ui.Error(err.Error())
		return 1
	}

	// This is our default message added to scaling submissions.
	msg := "submitted using the Nomad CLI"

	// Perform the scaling action.
	w := &api.WriteOptions{Namespace: namespace}
	req := &api.ScalingRequest{
		Count: pointer.Of(int64(count)),
		Target: map[string]string{
			"Job":   jobID,
			"Group": groupString,
		},
		Message:        msg,
		PolicyOverride: false,
		JobModifyIndex: checkIndex,
	}

	resp, _, err := client.Jobs().ScaleWithRequest(jobID, req, w)
	if err != nil {
		j.Ui.Error(fmt.Sprintf("Error submitting scaling request: %s", err))
		return 1
	}

	// Print any warnings if we have some.
	if resp.Warnings != "" {
		j.Ui.Output(
			j.Colorize().Color(fmt.Sprintf("[bold][yellow]Job Warnings:\n%s[reset]\n", resp.Warnings)))
	}

	jobInfo, _, err := client.Jobs().Info(jobID, q)
	if err != nil {
		j.Ui.Error(fmt.Sprintf("Error looking up job: %s", err))
		return 1
	}

	// Check if the job is periodic or is a parameterized job
	isPeriodicJob := jobInfo.IsPeriodic()
	isParameterizedJob := jobInfo.IsParameterized()
	isMultiregionJob := jobInfo.IsMultiregion()

	// Check if we should enter monitor mode
	if detach || isPeriodicJob || isParameterizedJob || isMultiregionJob {
		j.Ui.Output("Job scale successful")
		if isPeriodicJob && !isParameterizedJob {
			loc, err := jobInfo.Periodic.GetLocation()
			if err == nil {
				now := time.Now().In(loc)
				next, err := jobInfo.Periodic.Next(now)
				if err != nil {
					j.Ui.Error(fmt.Sprintf("Error determining next launch time: %v", err))
				} else {
					j.Ui.Output(fmt.Sprintf("Approximate next launch time: %s (%s from now)",
						formatTime(next), formatTimeDifference(now, next, time.Second)))
				}
			}
		} else if !isParameterizedJob {
			j.Ui.Output("Evaluation ID: " + resp.EvalID)
		}

		return 0
	}

	// Truncate the ID unless full length is requested.
	length := shortId
	if verbose {
		length = fullId
	}

	// Detach was not specified, so start monitoring.
	mon := newMonitor(j.Ui, client, length)
	return mon.monitor(resp.EvalID)
}

// performGroupCheck performs logic to ensure the user specified the correct
// group argument.
func (j *JobScaleCommand) performGroupCheck(groups map[string]api.TaskGroupScaleStatus, group *string) error {

	// If the job contains multiple groups and the user did not supply a task
	// group, return an error.
	if len(groups) > 1 && *group == "" {
		return errors.New("Group name required")
	}

	// We have to iterate the map to have any idea what task groups we are
	// dealing with.
	for groupName := range groups {

		// If the job has a single task group, and the user did not supply a
		// task group, it is assumed we scale the only group in the job.
		if len(groups) == 1 && *group == "" {
			*group = groupName
			return nil
		}

		// If we found a match, return.
		if groupName == *group {
			return nil
		}
	}

	// If we got here, we didn't find a match and therefore return an error.
	return fmt.Errorf("Group %v not found within job", *group)
}
