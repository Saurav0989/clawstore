package cmd

import (
	"context"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newEntityCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "entity",
		Short: "Manage entities",
	}
	cmd.AddCommand(newEntityListCmd())
	cmd.AddCommand(newEntityCreateCmd())
	cmd.AddCommand(newEntityShowCmd())
	cmd.AddCommand(newEntityDeleteCmd())
	return cmd
}

func newEntityListCmd() *cobra.Command {
	var typeFilter string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List entities",
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := newRuntimeDeps()
			if err != nil {
				return fatalErr(cmd, err)
			}
			defer deps.Close()

			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()
			entities, err := deps.DB.ListEntities(ctx, typeFilter)
			if err != nil {
				return fatalErr(cmd, err)
			}
			if len(entities) == 0 {
				writeStdout(cmd, "No entities found.\n")
				return nil
			}
			for _, e := range entities {
				writeStdout(cmd, "- %s\t%s\t%s\tobs=%d\tupdated=%s\n", e.ID, e.Name, e.Type, e.ObservationCount, unixToLocal(e.UpdatedAt))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&typeFilter, "type", "", "filter by entity type")
	return cmd
}

func newEntityCreateCmd() *cobra.Command {
	var (
		name string
		typ  string
	)
	cmd := &cobra.Command{
		Use:   "create <entity-id>",
		Short: "Create an entity",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := newRuntimeDeps()
			if err != nil {
				return fatalErr(cmd, err)
			}
			defer deps.Close()

			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()
			entityID := strings.ToLower(strings.TrimSpace(args[0]))
			entity, err := deps.DB.CreateEntity(ctx, entityID, name, typ)
			if err != nil {
				return fatalErr(cmd, err)
			}
			writeStdout(cmd, "created: %s (%s) type=%s\n", entity.ID, entity.Name, entity.Type)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "display name")
	cmd.Flags().StringVar(&typ, "type", "general", "entity type")
	return cmd
}

func newEntityShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <entity-id>",
		Short: "Show entity details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := newRuntimeDeps()
			if err != nil {
				return fatalErr(cmd, err)
			}
			defer deps.Close()

			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()
			entity, err := deps.DB.GetEntity(ctx, strings.ToLower(strings.TrimSpace(args[0])))
			if err != nil {
				return fatalErr(cmd, err)
			}
			writeStdout(cmd, "id: %s\nname: %s\ntype: %s\ncreated_at: %s\nupdated_at: %s\n", entity.ID, entity.Name, entity.Type, unixToLocal(entity.CreatedAt), unixToLocal(entity.UpdatedAt))
			return nil
		},
	}
	return cmd
}

func newEntityDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <entity-id>",
		Short: "Delete an entity",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := newRuntimeDeps()
			if err != nil {
				return fatalErr(cmd, err)
			}
			defer deps.Close()

			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()
			entityID := strings.ToLower(strings.TrimSpace(args[0]))
			if err := deps.DB.DeleteEntity(ctx, entityID); err != nil {
				return fatalErr(cmd, err)
			}
			writeStdout(cmd, "deleted %s\n", entityID)
			return nil
		},
	}
	return cmd
}
