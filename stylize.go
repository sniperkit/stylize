package main

import (
	"bytes"
	"github.com/bradfitz/slice"
	"github.com/pkg/errors"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

type FormattingResult struct {
	FilePath     string
	FormatNeeded bool
	Patch        string
	Error        error
}

func fileIsExcluded(file string, excludeDirs []string) bool {
	for _, eDir := range excludeDirs {
		if filepath.HasPrefix(file, eDir) {
			return true
		}
	}
	return false
}

// Walks the given directory and sends all non-excluded files to the returned channel.
// @param rootDir absolute path to root directory
// @return file paths relative to rootDir
func iterateAllFiles(rootDir string, excludeDirs []string) <-chan string {
	files := make(chan string)

	go func() {
		defer close(files)
		filepath.Walk(rootDir, func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return nil
			}

			excludeDirs = append(excludeDirs, absPathOrDie(".git"), absPathOrDie(".hg"))
			if fi.IsDir() && fileIsExcluded(path, excludeDirs) {
				return filepath.SkipDir
			}

			if fi.IsDir() {
				return nil
			}

			relPath, _ := filepath.Rel(rootDir, path)
			files <- relPath

			return nil
		})
	}()

	return files
}

// Finds files that have been modified since the common ancestor of HEAD and
// diffbase and sends them onto the returned channel.
// @return file paths relative to rootDir
// TODO: if a config file changes, rerun formatting on *all relevant files
func iterateGitChangedFiles(rootDir string, excludeDirs []string, diffbase string) (<-chan string, error) {
	cmd := exec.Command("git", "--no-pager", "diff", "--name-only", diffbase)
	cmd.Dir = rootDir
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	// note: these paths are all relative to the git root directory
	changedFiles := strings.Split(out.String(), "\n")

	// find ancestor directory of rootDir that has the .git directory
	var gitRootOut, stderr bytes.Buffer
	cmd = exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Stdout = &gitRootOut
	cmd.Stderr = &stderr
	cmd.Dir = rootDir
	err = cmd.Run()
	if err != nil {
		return nil, errors.Wrap(err, stderr.String())
	}
	gitRoot := strings.Trim(gitRootOut.String(), "\n")

	files := make(chan string)
	go func() {
		defer close(files)

		for _, file := range changedFiles {
			absPath := filepath.Join(gitRoot, file)
			if fileIsExcluded(absPath, excludeDirs) {
				log.Printf("Excluding file: %s", absPath)
				continue
			}

			// get file path relative to root directory
			relPath, err := filepath.Rel(rootDir, absPath)
			if err != nil {
				log.Fatal(err)
			}

			files <- relPath
		}
	}()

	return files, nil
}

func runFormatter(rootDir, file string, formatter Formatter, inPlace bool) FormattingResult {
	result := FormattingResult{
		FilePath: file,
	}

	if inPlace {
		result.FormatNeeded, result.Error = FormatInPlaceAndCheckModified(formatter, filepath.Join(rootDir, file))
	} else {
		result.Patch, result.Error = CreatePatchWithFormatter(formatter, rootDir, file)
		if len(result.Patch) > 0 {
			result.FormatNeeded = true
		}
	}

	return result
}

// Reads all incoming results and forwards them to the output channel. When all
// results have been read, writes the patch to the output writer.
func CollectPatch(results <-chan FormattingResult, patchOut io.Writer) <-chan FormattingResult {
	var wg sync.WaitGroup
	wg.Add(1)

	resultsOut := make(chan FormattingResult)

	go func() {
		defer close(resultsOut)

		// collect relevant results from the input channel and forward them to the output
		var resultList []FormattingResult
		for r := range results {
			if r.Error == nil && r.FormatNeeded {
				resultList = append(resultList, r)
			}
			resultsOut <- r
		}

		// sort to ensure patches are consistent
		slice.Sort(resultList, func(i, j int) bool {
			return resultList[i].FilePath < resultList[j].FilePath
		})

		// write patch output
		for _, r := range resultList {
			patchOut.Write([]byte(r.Patch))
		}
	}()

	return resultsOut
}

func RunFormattersOnFiles(formatters map[string]Formatter, fileChan <-chan string, rootDir string, inPlace bool, parallelism int) <-chan FormattingResult {
	semaphore := make(chan int, parallelism)
	var wg sync.WaitGroup

	resultOut := make(chan FormattingResult)
	go func() {
		for file := range fileChan {
			ext := filepath.Ext(file)
			formatter := formatters[ext]
			if formatter == nil {
				continue
			}

			wg.Add(1)
			semaphore <- 0 // acquire
			go func(file string, formatter Formatter, inPlace bool) {
				resultOut <- runFormatter(rootDir, file, formatter, inPlace)
				wg.Done()
				<-semaphore // release
			}(file, formatter, inPlace)
		}

		wg.Wait()
		close(resultOut)
	}()

	return resultOut
}

// @return (uglyCount, totalCount)
func StylizeMain(rootDir string, excludeDirs []string, gitDiffbase string, patchOut io.Writer, inPlace bool, parallelism int) (int, int) {
	if inPlace && patchOut != nil {
		log.Fatal("Patch output writer should only be provided in non-inplace runs")
	}
	if !filepath.IsAbs(rootDir) {
		log.Fatalf("root directory be an absolute path: '%s'", rootDir)
	}

	for _, excl := range excludeDirs {
		if !filepath.IsAbs(excl) {
			log.Fatal("exclude directories should be absolute")
		}
	}

	// setup file source
	var err error
	var fileChan <-chan string
	if len(gitDiffbase) > 0 {
		fileChan, err = iterateGitChangedFiles(rootDir, excludeDirs, gitDiffbase)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		fileChan = iterateAllFiles(rootDir, excludeDirs)
	}

	// run formatter on all files
	formatters := loadFormatters()
	results := RunFormattersOnFiles(formatters, fileChan, rootDir, inPlace, parallelism)

	// write patch to output if requested
	if patchOut != nil {
		results = CollectPatch(results, patchOut)
	}

	// iterate through all results, collecting basic stats and logging actions.
	uglyCount, totalCount := 0, 0
	for r := range results {
		totalCount++

		if r.Error != nil {
			log.Printf("Error formatting file '%s': %q\n", r.FilePath, r.Error)
			continue
		}

		if r.FormatNeeded {
			uglyCount++

			if inPlace {
				log.Printf("Formatted: '%s'", r.FilePath)
			} else {
				log.Printf("Needs formatting: '%s'", r.FilePath)
			}
		}
	}

	if inPlace {
		log.Printf("%d / %d formatted\n", uglyCount, totalCount)
	} else {
		log.Printf("%d / %d files need formatting\n", uglyCount, totalCount)
	}

	return uglyCount, totalCount
}