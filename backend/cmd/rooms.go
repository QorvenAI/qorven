// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/qorvenai/qorven/cmd/output"
	"github.com/qorvenai/qorven/cmd/tui"
)

var roomsCmd = &cobra.Command{
	Use:   "rooms",
	Short: "Manage chat rooms",
}

var roomsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List rooms",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		data, err := c.Get("/v1/rooms")
		if err != nil {
			return err
		}
		if cfg.OutputFormat != "table" {
			printer.Print(unmarshalList(data))
			return nil
		}
		tbl := output.NewTable("ID", "NAME", "DESCRIPTION", "MEMBERS", "MESSAGES")
		for _, r := range unmarshalList(data) {
			tbl.AddRow(
				str(r, "id"),
				str(r, "display_name"),
				str(r, "description"),
				str(r, "member_count"),
				str(r, "message_count"),
			)
		}
		printer.Print(tbl)
		return nil
	},
}

var roomsGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get room details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		data, err := c.Get("/v1/rooms/" + args[0])
		if err != nil {
			return err
		}
		printer.Print(unmarshalMap(data))
		return nil
	},
}

var roomsMessagesCmd = &cobra.Command{
	Use:   "messages <id>",
	Short: "View room messages",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		data, err := c.Get("/v1/rooms/" + args[0] + "/messages")
		if err != nil {
			return err
		}
		if cfg.OutputFormat != "table" {
			printer.Print(unmarshalList(data))
			return nil
		}
		var msgs []struct {
			SenderType string `json:"sender_type"`
			Content    string `json:"content"`
			CreatedAt  string `json:"created_at"`
		}
		json.Unmarshal(data, &msgs)
		for _, m := range msgs {
			content := m.Content
			if len(content) > 100 {
				content = content[:100] + "..."
			}
			ts := m.CreatedAt
			if len(ts) > 16 {
				ts = ts[:16]
			}
			fmt.Printf("[%s] %s: %s\n", ts, m.SenderType, content)
		}
		return nil
	},
}

var roomsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a room",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		name, _ := cmd.Flags().GetString("name")
		desc, _ := cmd.Flags().GetString("description")
		body := buildBody("name", name, "display_name", name, "description", desc)
		data, err := c.Post("/v1/rooms", body)
		if err != nil {
			return err
		}
		m := unmarshalMap(data)
		printer.Success(fmt.Sprintf("Room created: %s (ID: %s)", str(m, "display_name"), str(m, "id")))
		return nil
	},
}

var roomsDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a room",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !tui.Confirm(fmt.Sprintf("Delete room %s?", args[0]), cfg.Yes) {
			return nil
		}
		c, err := newHTTP()
		if err != nil {
			return err
		}
		_, err = c.Delete("/v1/rooms/" + args[0])
		if err != nil {
			return err
		}
		printer.Success("Room deleted")
		return nil
	},
}

func init() {
	roomsCreateCmd.Flags().String("name", "", "Room name")
	roomsCreateCmd.Flags().String("description", "", "Room description")
	_ = roomsCreateCmd.MarkFlagRequired("name")

	roomsCmd.AddCommand(roomsListCmd, roomsGetCmd, roomsMessagesCmd, roomsCreateCmd, roomsDeleteCmd)
	rootCmd.AddCommand(roomsCmd)
}
