package main

import (
	"fmt"

	"github.com/codegangsta/cli"
	"github.com/leancloud/lean-cli/lean/api"
	"github.com/leancloud/lean-cli/lean/apps"
)

func checkOutAction(c *cli.Context) error {
	if c.NArg() > 0 {
		appID := c.Args()[0]
		fmt.Println("切换至应用：" + appID)
		err := apps.LinkApp("", appID)
		if err != nil {
			return newCliError(err)
		}
		return nil
	}

	region, err := selectRegion()
	if err != nil {
		return newCliError(err)
	}

	op.Write("获取应用列表")
	appList, err := api.GetAppList(region)
	if err != nil {
		op.Failed()
		return newCliError(err)
	}
	op.Successed()

	appList, err = apps.MergeWithRecentApps(".", appList)
	if err != nil {
		return newCliError(err)
	}

	// remove current linked app from app list
	curentAppID, err := apps.GetCurrentAppID(".")
	if err != nil {
		if err != apps.ErrNoAppLinked {
			return newCliError(err)
		}
	} else {
		for i, app := range appList {
			if app.AppID == curentAppID {
				appList = append(appList[:i], appList[i+1:]...)
			}
		}
	}

	app, err := selectApp(appList)
	if err != nil {
		return newCliError(err)
	}
	fmt.Println("切换应用至 " + app.AppName)

	err = apps.LinkApp(".", app.AppID)
	if err != nil {
		return newCliError(err)
	}
	return nil
}
