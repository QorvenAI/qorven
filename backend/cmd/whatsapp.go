package cmd

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

var whatsappCmd = &cobra.Command{
	Use:   "whatsapp",
	Short: "WhatsApp bridge utilities",
}

var whatsappQRCmd = &cobra.Command{
	Use:   "qr <channel-id>",
	Short: "Display WhatsApp pairing QR in terminal",
	Long: `Stream the WhatsApp QR code for the specified channel and render it
in the terminal using block characters. Press Ctrl+C to exit.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		channelID := args[0]
		c, err := newHTTP()
		if err != nil {
			return err
		}

		fmt.Println("Connecting to WhatsApp bridge for channel", channelID, "...")
		fmt.Println("Press Ctrl+C to exit")
		fmt.Println()

		resp, err := c.GetRaw("/v1/channels/" + channelID + "/whatsapp/qr")
		if err != nil {
			return fmt.Errorf("QR stream: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			return fmt.Errorf("server returned HTTP %d", resp.StatusCode)
		}

		return streamQRToTerminal(resp.Body)
	},
}

func streamQRToTerminal(r io.Reader) error {
	var lastQRLines int
	buf := make([]byte, 4096)
	var accumulated string

	for {
		n, err := r.Read(buf)
		if n > 0 {
			accumulated += string(buf[:n])
			for {
				idx := strings.Index(accumulated, "\n\n")
				if idx < 0 {
					break
				}
				line := accumulated[:idx]
				accumulated = accumulated[idx+2:]

				if !strings.HasPrefix(line, "data: ") {
					continue
				}
				data := strings.TrimPrefix(line, "data: ")
				if data == "" || strings.HasPrefix(data, ":") {
					continue
				}

				qrB64 := extractQRBase64(data)
				if qrB64 == "" {
					continue
				}

				imgData, decErr := base64.StdEncoding.DecodeString(qrB64)
				if decErr != nil {
					continue
				}

				img, _, decErr := image.Decode(bytes.NewReader(imgData))
				if decErr != nil {
					// Try PNG specifically
					img, decErr = png.Decode(bytes.NewReader(imgData))
					if decErr != nil {
						continue
					}
				}

				if lastQRLines > 0 {
					fmt.Printf("\x1b[%dA\x1b[0J", lastQRLines)
				}

				rendered := renderQRImage(img)
				fmt.Print(rendered)
				lastQRLines = strings.Count(rendered, "\n") + 1
			}
		}
		if err == io.EOF {
			fmt.Println("\nStream ended.")
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// renderQRImage converts a decoded QR PNG image to ANSI block chars.
func renderQRImage(img image.Image) string {
	bounds := img.Bounds()
	w := bounds.Max.X - bounds.Min.X
	h := bounds.Max.Y - bounds.Min.Y

	bitmap := make([][]bool, h)
	for y := 0; y < h; y++ {
		bitmap[y] = make([]bool, w)
		for x := 0; x < w; x++ {
			r, g, b, _ := img.At(x+bounds.Min.X, y+bounds.Min.Y).RGBA()
			bitmap[y][x] = (r+g+b)/3 < 0x8000
		}
	}
	return renderQRBitmap(bitmap)
}

// renderQRBitmap renders a boolean bitmap as ANSI block characters.
// Each output row represents 2 bitmap rows using ▀ ▄ █ and space.
func renderQRBitmap(bitmap [][]bool) string {
	if len(bitmap) == 0 {
		return ""
	}
	h := len(bitmap)
	w := len(bitmap[0])
	var sb strings.Builder

	for y := 0; y < h; y += 2 {
		for x := 0; x < w; x++ {
			top := bitmap[y][x]
			bot := false
			if y+1 < h {
				bot = bitmap[y+1][x]
			}
			switch {
			case top && bot:
				sb.WriteRune('█')
			case top:
				sb.WriteRune('▀')
			case bot:
				sb.WriteRune('▄')
			default:
				sb.WriteRune(' ')
			}
		}
		sb.WriteRune('\n')
	}
	return sb.String()
}

// extractQRBase64 parses {"type":"qr","qr":"data:image/png;base64,<DATA>"} and returns <DATA>.
func extractQRBase64(jsonStr string) string {
	const prefix = `"data:image/png;base64,`
	idx := strings.Index(jsonStr, prefix)
	if idx < 0 {
		return ""
	}
	rest := jsonStr[idx+len(prefix):]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return ""
	}
	return rest[:end]
}

func init() {
	whatsappCmd.AddCommand(whatsappQRCmd)
	channelsCmd.AddCommand(whatsappCmd)
}
