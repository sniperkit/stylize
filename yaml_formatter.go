package main

import (
	"bytes"
	"gopkg.in/yaml.v2"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
)

type YamlFormatter struct{}

func init() {
	RegisterFormatter("yaml", &YamlFormatter{})
}

func (F *YamlFormatter) FileExtensions() []string {
	return []string{".yml"}
}

func (F *YamlFormatter) IsInstalled() bool {
	_, err := exec.LookPath("buildifier")
	return err == nil
}

func (F *YamlFormatter) FormatToBuffer(in io.Reader, out io.Writer) error {
	data, err := ioutil.ReadAll(in)
	if err != nil {
		log.Fatal(err)
	}

	t := yaml.MapSlice{}

	err = yaml.Unmarshal([]byte(data), &t)
	if err != nil {
		return err
	}

	d, err := yaml.Marshal(&t)
	if err != nil {
		return err
	}

	// write formatted yml to output
	_, err = out.Write(d)
	if err != nil {
		return err
	}

	return nil
}

func (F *YamlFormatter) FormatInPlace(file string) error {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}

	out, err := os.Create(file)
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()

	return F.FormatToBuffer(bytes.NewReader(data), out)
}
