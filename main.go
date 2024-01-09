package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"

	"github.com/alessio/shellescape"
	"github.com/spf13/cobra"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func main() {
	root := cobra.Command{
		Use: "obex2drawio",
	}
	root.AddCommand(extractCommand(), convertCommand())

	if err := root.Execute(); err != nil {
		panic(err)
	}
}

func convertCommand() *cobra.Command {
	var (
		clipFlag  bool
		deleteTmp bool
	)
	cmd := &cobra.Command{
		Use:   "convert",
		Short: "convert excalidraw.md in obsidian to gliffy(supporting drawio), and copy the converted data to the clipboard(using pbcopy on MacOS)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			mdSourceFile := args[0]
			excalidrawJsonFile, err := os.CreateTemp("", "obex2drawio.json.*")
			if err != nil {
				panic(fmt.Errorf("create source tempfile failed: %v", err))
			}
			excalidrawJsonFile.Close()
			out, err := exec.Command("sh", "-c", fmt.Sprintf("obex2drawio extract %v > %v", shellescape.Quote(mdSourceFile), shellescape.Quote(excalidrawJsonFile.Name()))).CombinedOutput()
			if err != nil {
				panic(fmt.Errorf("extract excalidraw data failed: %s", out))
			}
			if deleteTmp {
				defer os.Remove(excalidrawJsonFile.Name())
			}

			gliffyFile, err := os.CreateTemp("", "obex2drawio.gliffy.*")
			if err != nil {
				panic(fmt.Errorf("create out tempfile failed: %v", err))
			}
			gliffyFile.Close()
			if deleteTmp {
				defer os.Remove(gliffyFile.Name())
			}
			out, err = exec.Command("exconv", "gliffy", "-i", excalidrawJsonFile.Name(), "-o", gliffyFile.Name()).CombinedOutput()
			if err != nil {
				panic(fmt.Errorf("convert failed: %s", out))
			}
			if clipFlag && runtime.GOOS == "darwin" {
				out, err = exec.Command("sh", "-c", fmt.Sprintf("cat %v | pbcopy", shellescape.Quote(gliffyFile.Name()))).CombinedOutput()
				if err != nil {
					panic(fmt.Errorf("convert failed: %s", out))
				}
			} else {
				data, err := os.ReadFile(gliffyFile.Name())
				if err != nil {
					panic(fmt.Errorf("read gliffy file failed: %v", err))
				}
				fmt.Println(string(data))
			}
		},
	}

	cmd.Flags().BoolVar(&clipFlag, "clip", true, "copy to clipboard")
	cmd.Flags().BoolVar(&deleteTmp, "delete", true, "delete tmpfiles")

	return cmd
}

func extractCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "extract FILE",
		Short: "extract raw excalidraw json data from excalidraw.md",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			file := args[0]
			var r io.Reader
			var err error
			if file == "-" {
				r = os.Stdin
			} else {
				r, err = os.Open(file)
				if err != nil {
					panic(fmt.Errorf("open file failed: %v", err))
				}
			}
			err = extract(os.Stdout, r)
			if err != nil {
				panic(fmt.Errorf("extract failed: %v", err))
			}
		},
	}
	return cmd
}

func extract(dst io.Writer, src io.Reader) error {
	source, err := io.ReadAll(src)
	if err != nil {
		return fmt.Errorf("read from src failed: %w", err)
	}
	markdown := goldmark.New()
	doc := markdown.Parser().Parse(text.NewReader(source))

	var excalidrawJson bytes.Buffer
	count := 0
	err = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if n.Kind() == ast.KindFencedCodeBlock {
			n := n.(*ast.FencedCodeBlock)
			if entering {
				if string(n.Language(source)) == "json" {
					count++
					l := n.Lines().Len()
					for i := 0; i < l; i++ {
						line := n.Lines().At(i)
						excalidrawJson.Write(line.Value(source))
					}
				}
			}
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return fmt.Errorf("walk markdown failed: %w", err)
	}
	if count != 1 {
		return fmt.Errorf("invalid number of json blocks: %v", count)
	}

	_, err = io.Copy(dst, &excalidrawJson)
	if err != nil {
		return fmt.Errorf("write to dst failed: %w", err)
	}
	return nil
}
