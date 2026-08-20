package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// WorkspaceHarnessCase 持久化在 runtime/harness-cases.jsonl 的评测用例
type WorkspaceHarnessCase struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Goal           string `json:"goal,omitempty"`
	Kind           string `json:"kind,omitempty"`
	Command        string `json:"command,omitempty"`
	ExpectedResult string `json:"expectedResult,omitempty"`
	Source         string `json:"source,omitempty"`
	FilePath       string `json:"filePath,omitempty"`
	PromptName     string `json:"promptName,omitempty"`
	InputText      string `json:"inputText,omitempty"`
	CreatedAt      int64  `json:"createdAt"`
	UpdatedAt      int64  `json:"updatedAt"`
}

func (s *Server) harnessCasesPath() string {
	return filepath.Join(s.workspace, "runtime", "harness-cases.jsonl")
}

func (s *Server) loadHarnessCases() []WorkspaceHarnessCase {
	path := s.harnessCasesPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return []WorkspaceHarnessCase{}
	}
	file, err := os.Open(path)
	if err != nil {
		return []WorkspaceHarnessCase{}
	}
	defer file.Close()
	var cases []WorkspaceHarnessCase
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var testCase WorkspaceHarnessCase
		if json.Unmarshal(scanner.Bytes(), &testCase) == nil {
			cases = append(cases, testCase)
		}
	}
	return cases
}

func (s *Server) findHarnessCase(id string) (WorkspaceHarnessCase, bool) {
	id = strings.TrimSpace(id)
	for _, testCase := range s.loadHarnessCases() {
		if testCase.ID == id {
			return testCase, true
		}
	}
	return WorkspaceHarnessCase{}, false
}

func (s *Server) runWorkspaceHarnessCase(id string) (HarnessResult, error) {
	testCase, ok := s.findHarnessCase(id)
	if !ok {
		return HarnessResult{}, fmt.Errorf("评测用例未找到: %s", id)
	}
	if strings.TrimSpace(testCase.PromptName) == "" {
		return HarnessResult{}, fmt.Errorf("评测用例 %s 未绑定 prompt", id)
	}
	inputText := strings.TrimSpace(testCase.InputText)
	if inputText == "" {
		inputText = strings.TrimSpace(testCase.Goal)
	}
	if inputText == "" {
		inputText = strings.TrimSpace(testCase.ExpectedResult)
	}
	if inputText == "" {
		return HarnessResult{}, fmt.Errorf("评测用例 %s 缺少输入内容", id)
	}
	return s.runWorkspaceHarness(testCase.PromptName, inputText)
}

func (s *Server) createHarnessCase(record WorkspaceHarnessCase) (WorkspaceHarnessCase, error) {
	record.Title = strings.TrimSpace(record.Title)
	record.Goal = strings.TrimSpace(record.Goal)
	record.Kind = strings.TrimSpace(record.Kind)
	record.Command = strings.TrimSpace(record.Command)
	record.ExpectedResult = strings.TrimSpace(record.ExpectedResult)
	record.Source = strings.TrimSpace(record.Source)
	record.FilePath = strings.TrimSpace(record.FilePath)
	record.PromptName = strings.TrimSpace(record.PromptName)
	record.InputText = strings.TrimSpace(record.InputText)
	if record.Title == "" {
		return WorkspaceHarnessCase{}, fmt.Errorf("harness title 为必填参数")
	}
	now := time.Now().Unix()
	record.ID = fmt.Sprintf("harness-case-%d", time.Now().UnixNano())
	record.CreatedAt = now
	record.UpdatedAt = now

	runtimeDir := filepath.Join(s.workspace, "runtime")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		return WorkspaceHarnessCase{}, err
	}
	file, err := os.OpenFile(s.harnessCasesPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return WorkspaceHarnessCase{}, err
	}
	defer file.Close()
	data, err := json.Marshal(record)
	if err != nil {
		return WorkspaceHarnessCase{}, err
	}
	_, err = file.Write(append(data, '\n'))
	return record, err
}
