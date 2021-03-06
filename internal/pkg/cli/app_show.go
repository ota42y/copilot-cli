// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"fmt"
	"io"

	"github.com/aws/copilot-cli/internal/pkg/aws/codepipeline"
	"github.com/aws/copilot-cli/internal/pkg/aws/sessions"
	"github.com/aws/copilot-cli/internal/pkg/config"
	"github.com/aws/copilot-cli/internal/pkg/deploy"
	"github.com/aws/copilot-cli/internal/pkg/term/prompt"
	"github.com/aws/copilot-cli/internal/pkg/term/selector"

	"github.com/aws/copilot-cli/internal/pkg/describe"
	"github.com/aws/copilot-cli/internal/pkg/term/log"
	"github.com/spf13/cobra"
)

const (
	appShowNamePrompt     = "Which application would you like to show?"
	appShowNameHelpPrompt = "An application is a collection of related services."
)

type showAppVars struct {
	name             string
	shouldOutputJSON bool
}

type showAppOpts struct {
	showAppVars

	prompt      prompter
	store       store
	w           io.Writer
	sel         appSelector
	pipelineSvc pipelineGetter
}

func newShowAppOpts(vars showAppVars) (*showAppOpts, error) {
	store, err := config.NewStore()
	if err != nil {
		return nil, fmt.Errorf("new config store: %w", err)
	}

	defaultSession, err := sessions.NewProvider().Default()
	if err != nil {
		return nil, fmt.Errorf("default session: %w", err)
	}
	prompter := prompt.New()
	return &showAppOpts{
		showAppVars: vars,
		store:       store,
		w:           log.OutputWriter,
		prompt:      prompter,
		sel:         selector.NewSelect(prompter, store),
		pipelineSvc: codepipeline.New(defaultSession),
	}, nil
}

// Validate returns an error if the values provided by the user are invalid.
func (o *showAppOpts) Validate() error {
	if o.name != "" {
		_, err := o.store.GetApplication(o.name)
		if err != nil {
			return fmt.Errorf("get application %s: %w", o.name, err)
		}
	}

	return nil
}

// Ask asks for fields that are required but not passed in.
func (o *showAppOpts) Ask() error {
	if err := o.askName(); err != nil {
		return err
	}

	return nil
}

// Execute writes the application's description.
func (o *showAppOpts) Execute() error {
	description, err := o.description()
	if err != nil {
		return err
	}
	if !o.shouldOutputJSON {
		fmt.Fprint(o.w, description.HumanString())
		return nil
	}
	data, err := description.JSONString()
	if err != nil {
		return fmt.Errorf("get JSON string: %w", err)
	}
	fmt.Fprint(o.w, data)
	return nil
}

func (o *showAppOpts) description() (*describe.App, error) {
	app, err := o.store.GetApplication(o.name)
	if err != nil {
		return nil, fmt.Errorf("get application %s: %w", o.name, err)
	}
	envs, err := o.store.ListEnvironments(o.name)
	if err != nil {
		return nil, fmt.Errorf("list environments in application %s: %w", o.name, err)
	}
	svcs, err := o.store.ListServices(o.name)
	if err != nil {
		return nil, fmt.Errorf("list services in application %s: %w", o.name, err)
	}

	pipelines, err := o.pipelineSvc.GetPipelinesByTags(map[string]string{
		deploy.AppTagKey: o.name,
	})

	if err != nil {
		return nil, fmt.Errorf("list pipelines in application %s: %w", o.name, err)
	}

	var trimmedEnvs []*config.Environment
	for _, env := range envs {
		trimmedEnvs = append(trimmedEnvs, &config.Environment{
			Name:      env.Name,
			AccountID: env.AccountID,
			Region:    env.Region,
			Prod:      env.Prod,
		})
	}
	var trimmedSvcs []*config.Workload
	for _, svc := range svcs {
		trimmedSvcs = append(trimmedSvcs, &config.Workload{
			Name: svc.Name,
			Type: svc.Type,
		})
	}
	return &describe.App{
		Name:      app.Name,
		URI:       app.Domain,
		Envs:      trimmedEnvs,
		Services:  trimmedSvcs,
		Pipelines: pipelines,
	}, nil
}

func (o *showAppOpts) askName() error {
	if o.name != "" {
		return nil
	}
	name, err := o.sel.Application(appShowNamePrompt, appShowNameHelpPrompt)
	if err != nil {
		return fmt.Errorf("select application: %w", err)
	}
	o.name = name
	return nil
}

// buildAppShowCmd builds the command for showing details of an application.
func buildAppShowCmd() *cobra.Command {
	vars := showAppVars{}
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Shows info about an application.",
		Long:  "Shows configuration, environments and services for an application.",
		Example: `
  Shows info about the application "my-app"
  /code $ copilot app show -n my-app`,
		RunE: runCmdE(func(cmd *cobra.Command, args []string) error {
			opts, err := newShowAppOpts(vars)
			if err != nil {
				return err
			}
			if err := opts.Validate(); err != nil {
				return err
			}
			if err := opts.Ask(); err != nil {
				return err
			}
			if err := opts.Execute(); err != nil {
				return err
			}

			return nil
		}),
	}
	// The flags bound by viper are available to all sub-commands through viper.GetString({flagName})
	cmd.Flags().BoolVar(&vars.shouldOutputJSON, jsonFlag, false, jsonFlagDescription)
	cmd.Flags().StringVarP(&vars.name, nameFlag, nameFlagShort, tryReadingAppName(), appFlagDescription)
	return cmd
}
