package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/ahmetalpbalkan/go-linq"
	"github.com/codegangsta/cli"
	"github.com/facebookgo/parseignore"
	"github.com/fatih/color"
	"github.com/jhoonb/archivex"
	"github.com/leancloud/lean-cli/lean/api"
	"github.com/leancloud/lean-cli/lean/apps"
	"github.com/leancloud/lean-cli/lean/runtimes"
	"github.com/leancloud/lean-cli/lean/utils"
)

func determineGroupName(appID string) (string, error) {
	op.Write("获取应用信息")
	info, err := api.GetAppInfo(appID)
	if err != nil {
		op.Failed()
		return "", err
	}
	op.Successed()
	fmt.Println("> 准备部署至目标应用：" + color.RedString(info.AppName) + " (" + appID + ")")
	mode := info.LeanEngineMode

	op.Write("获取应用分组信息")
	groups, err := api.GetGroups(appID)
	if err != nil {
		op.Failed()
		return "", err
	}
	op.Successed()

	groupName, found, err := linq.From(groups).Where(func(group linq.T) (bool, error) {
		groupName := group.(*api.GetGroupsResult).GroupName
		if mode == "free" {
			return groupName != "staging", nil
		}
		return groupName == "staging", nil
	}).Select(func(group linq.T) (linq.T, error) {
		return group.(*api.GetGroupsResult).GroupName, nil
	}).First()
	if err != nil {
		return "", err
	}
	if !found {
		return "", errors.New("group not found")
	}
	return groupName.(string), nil
}

func readIgnore(ignoreFilePath string) (parseignore.Matcher, error) {
	content, err := ioutil.ReadFile(ignoreFilePath)
	if err != nil {
		return nil, err
	}

	matcher, errs := parseignore.CompilePatterns(content)
	if len(errs) != 0 {
		return nil, errs[0]
	}

	return matcher, nil
}

func uploadProject(appID string, repoPath string, ignoreFilePath string) (*api.UploadFileResult, error) {
	fileDir, err := ioutil.TempDir("", "leanengine")
	if err != nil {
		return nil, err
	}

	filePath := filepath.Join(fileDir, "leanengine.zip")

	runtime, err := runtimes.DetectRuntime(repoPath)
	if err != nil {
		return nil, err
	}

	if ignoreFilePath == ".leanignore" && !utils.IsFileExists(filepath.Join(repoPath, ".leanignore")) {
		fmt.Println("> 没有找到 .leanignore 文件，根据项目文件创建默认的 .leanignore 文件")
		content := strings.Join(runtime.DefaultIgnorePatterns(), "\r\n")
		err := ioutil.WriteFile(filepath.Join(repoPath, ".leanignore"), []byte(content), 0644)
		if err != nil {
			return nil, err
		}
	}

	matcher, err := readIgnore(ignoreFilePath)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("指定的 ignore 文件 '%s' 不存在", ignoreFilePath)
	} else if err != nil {
		return nil, err
	}

	files := []string{}
	filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		decision, err := matcher.Match(path, info)
		if err != nil {
			return err
		}
		if decision != parseignore.Exclude {
			files = append(files, path)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	op.Write("压缩项目文件 ...")
	zip := new(archivex.ZipFile)
	func() {
		defer zip.Close()
		zip.Create(filePath)
		for _, f := range files {
			err := zip.AddFile(filepath.ToSlash(f))
			if err != nil {
				panic(err)
			}
		}
	}()
	op.Successed()

	file, err := api.UploadFile(appID, filePath)
	if err != nil {
		return nil, err
	}

	return file, nil
}

func deployFromLocal(appID string, groupName string, ignoreFilePath string, message string) error {
	file, err := uploadProject(appID, ".", ignoreFilePath)
	if err != nil {
		return err
	}

	defer func() {
		op.Write("删除临时文件")
		err := api.DeleteFile(appID, file.ObjectID)
		if err != nil {
			op.Failed()
		} else {
			op.Successed()
		}
	}()

	eventTok, err := api.DeployAppFromFile(appID, ".", groupName, file.URL, message)
	ok, err := api.PollEvents(appID, eventTok, os.Stdout)
	if err != nil {
		return err
	}
	if !ok {
		return cli.NewExitError("部署失败", 1)
	}
	return nil
}

func deployFromGit(appID string, groupName string) error {
	eventTok, err := api.DeployAppFromGit(appID, ".", groupName)
	if err != nil {
		return err
	}
	ok, err := api.PollEvents(appID, eventTok, os.Stdout)
	if err != nil {
		return err
	}
	if !ok {
		return cli.NewExitError("部署失败", 1)
	}
	return nil
}

func deployAction(c *cli.Context) error {
	isDeployFromGit := c.Bool("g")
	ignoreFilePath := c.String("leanignore")
	message := c.String("message")

	appID, err := apps.GetCurrentAppID("")
	if err == apps.ErrNoAppLinked {
		return cli.NewExitError("没有关联任何 app，请使用 lean checkout 来关联应用。", 1)
	}
	if err != nil {
		return newCliError(err)
	}

	groupName, err := determineGroupName(appID)
	if err != nil {
		op.Failed()
		return newCliError(err)
	}

	if groupName == "staging" {
		fmt.Println("> 准备部署应用到预备环境")
	} else {
		fmt.Println("> 准备部署应用到生产环境: " + groupName)
	}

	if isDeployFromGit {
		err = deployFromGit(appID, groupName)
		if err != nil {
			return newCliError(err)
		}
	} else {
		err = deployFromLocal(appID, groupName, ignoreFilePath, message)
		if err != nil {
			return newCliError(err)
		}
	}
	return nil
}
