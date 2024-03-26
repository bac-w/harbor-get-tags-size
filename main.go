package main

import (
	"context"
	"fmt"
	"github.com/goharbor/go-client/pkg/harbor"
	"github.com/goharbor/go-client/pkg/sdk/v2.0/client/artifact"
	"github.com/goharbor/go-client/pkg/sdk/v2.0/client/repository"
	"github.com/goharbor/go-client/pkg/sdk/v2.0/models"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/schollz/progressbar/v3"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"math"
	"net/url"
	"os"
	"strconv"
	"strings"
)

var defaultCountElements = int64(100)
var debug bool
var username, password, host, projectName string
var sortAsc, sortDsc, progress bool
var version = "1.0.0"

var rootCmd = &cobra.Command{
	Use:           "hartisize",
	Short:         "hartisize – cli interface for get size artifacts in harbor project",
	Long:          `Get all repositories and all artifacts in harbor project and print size of`,
	Version:       version,
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		execute()
		return
	},
}

type artifactsSize struct {
	countTags      int
	artifactSize   int64
	repositoryName string
	tags           []string
}

func init() {
	log.SetFormatter(&log.TextFormatter{
		ForceColors: true,
	})

	log.SetOutput(os.Stdout)
	log.SetLevel(log.InfoLevel)
	traceEnv, traceEnvEx := os.LookupEnv("HB_SIZE_TRACE")
	if traceEnvEx {
		trace, err := strconv.ParseBool(traceEnv)
		if err != nil {
			log.Fatal(err)
		}
		if trace {
			log.SetLevel(log.DebugLevel)
			log.SetReportCaller(trace)
		}
	}
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Debug hartisize")
	rootCmd.PersistentFlags().StringVar(&projectName, "project", "myProject", "Set project name")
	rootCmd.PersistentFlags().StringVar(&username, "username", "Admin", "Username for harbor account")
	rootCmd.PersistentFlags().StringVar(&password, "password", "Password", "Password for harbor account")
	rootCmd.PersistentFlags().StringVar(&host, "host", "https://localhost", "Harbor host")
	rootCmd.PersistentFlags().BoolVar(&sortAsc, "sortAsc", false, "Sort by size min-max")
	rootCmd.PersistentFlags().BoolVar(&sortDsc, "sortDsc", false, "Sort by size max-min")
	rootCmd.PersistentFlags().BoolVar(&progress, "progress", true, "Show progress bar")
}

func main() {
	err := rootCmd.Execute()
	if err != nil {
		log.Fatal(err)
	}
}

func execute() {
	urlObj, err := url.Parse(host)
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.TODO()
	c := harbor.ClientSetConfig{
		URL:      urlObj.String(),
		Username: username,
		Password: password,
	}
	cs, err := harbor.NewClientSet(&c)
	artifacts, err := getAllArtifacts(cs, ctx, "advertising")
	if err != nil {
		log.Fatal(err.Error())
	}
	tw := table.NewWriter()
	tw.SetStyle(table.StyleColoredDark)
	tw.SetTitle(fmt.Sprintf("Harbor artifacts size of project - %s", projectName))
	tw.AppendHeader(table.Row{
		"#",
		"Repository",
		"CountTags",
		"Size",
		"SizeInt",
	})
	tw.SetColumnConfigs([]table.ColumnConfig{
		{Name: "Dark", Align: text.AlignCenter, AlignHeader: text.AlignCenter},
	})
	tw.Style().Title.Align = text.AlignCenter
	if sortAsc || sortDsc {
		var sortBy table.SortMode
		if sortAsc {
			sortBy = table.AscNumeric
		} else if sortDsc {
			sortBy = table.DscNumeric
		}
		tw.SortBy([]table.SortBy{{Name: "SizeInt", Mode: sortBy}})
	}
	var total int64
	for k, v := range artifacts {
		tw.AppendRow(table.Row{
			k,
			v.repositoryName,
			v.countTags,
			humanArtifactSize(v.artifactSize),
			v.artifactSize,
		})
		total += v.artifactSize
	}
	tw.AppendFooter(table.Row{
		"ArtifactsCount",
		fmt.Sprintf("%v", len(artifacts)),
		"TotalSize",
		humanArtifactSize(total),
	})

	tw.SetColumnConfigs([]table.ColumnConfig{{
		Name:   "SizeInt",
		Hidden: true,
	}})
	fmt.Println(tw.Render())
}

func getRepos(cs *harbor.ClientSet, ctx context.Context, projectName string) (repos []*models.Repository, err error) {
	log.Debugf("try get repos for %s project", projectName)
	var repoCount int
	repoCount, err = getCountElements(cs, ctx, "repoList", projectName, "")
	if err != nil {
		return
	}
	for i := 1; i <= repoCount; i++ {
		var repo *repository.ListRepositoriesOK
		count := int64(i)
		repo, err = getRepositoryList(cs, ctx, projectName, &defaultCountElements, &count)
		if err != nil {
			return
		}
		repos = append(repos, repo.Payload...)
	}
	return
}

func getRepositoryList(cs *harbor.ClientSet, ctx context.Context, projectName string, count *int64, page *int64) (repoList *repository.ListRepositoriesOK, err error) {
	params := &repository.ListRepositoriesParams{
		ProjectName: projectName,
		PageSize:    count,
		Page:        page,
	}
	repoList, err = cs.V2().Repository.ListRepositories(ctx, params)
	return
}

func getCountElements(cs *harbor.ClientSet, ctx context.Context, typeElements string, projectName string, repoName string) (count int, err error) {
	log.Debugf("try get count elements for %s type", typeElements)
	switch typeElements {
	case "artifactList":
		var res *artifact.ListArtifactsOK
		params := artifact.NewListArtifactsParams().WithProjectName(projectName).WithRepositoryName(url.QueryEscape(strings.TrimPrefix(repoName, fmt.Sprintf("%v/", projectName))))
		res, err = cs.V2().Artifact.ListArtifacts(ctx, params)
		if err != nil {
			return
		}
		if res.XTotalCount == 0 {
			count = 0
		} else {
			count = int(math.RoundToEven(float64(float64(res.XTotalCount)/float64(defaultCountElements)) + 0.6))
		}
		return
	case "repoList":
		var res *repository.ListRepositoriesOK
		res, err = cs.V2().Repository.ListRepositories(ctx, &repository.ListRepositoriesParams{ProjectName: projectName})
		if err != nil {
			return
		}
		if res.XTotalCount == 0 {
			count = 0
		} else {
			count = int(math.RoundToEven(float64(float64(res.XTotalCount)/float64(defaultCountElements)) + 0.6))
		}
		return
	}
	return
}

func getArtifactList(cs *harbor.ClientSet, ctx context.Context, projectName string, repoName string, count *int64, page *int64) (artifactList *artifact.ListArtifactsOK, err error) {
	tag := true
	params := artifact.NewListArtifactsParams().WithPage(page).WithPageSize(count).WithProjectName(projectName).WithRepositoryName(url.QueryEscape(strings.TrimPrefix(repoName, fmt.Sprintf("%v/", projectName)))).WithWithTag(&tag)
	log.Debugf("RepositoryName: %v", url.QueryEscape(strings.TrimPrefix(repoName, fmt.Sprintf("%v/", projectName))))
	artifactList, err = cs.V2().Artifact.ListArtifacts(ctx, params)
	return
}

func getAllArtifacts(cs *harbor.ClientSet, ctx context.Context, projectName string) (artifactList []*artifactsSize, err error) {
	var repos []*models.Repository
	repos, err = getRepos(cs, ctx, projectName)
	if err != nil {
		return
	}
	var bar *progressbar.ProgressBar
	if progress {
		bar = progressbar.NewOptions(len(repos),
			progressbar.OptionEnableColorCodes(true),
			progressbar.OptionOnCompletion(func() {
				fmt.Printf("\n")
			}),
			progressbar.OptionFullWidth())
	}
	for _, v := range repos {
		var artifactCount int
		artifactCount, err = getCountElements(cs, ctx, "artifactList", projectName, v.Name)
		if err != nil {
			return
		}
		if progress {
			bar.Describe(fmt.Sprintf("[green]🚀	%s [yellow]", v.Name))
			bar.Add(1)
		}
		if artifactCount == 0 {
			continue
		}
		oneArtifact := new(artifactsSize)
		log.Debugf("try get artifacts for %s project && %s repository", projectName, v.Name)
		oneArtifact.repositoryName = v.Name
		for i := 1; i <= artifactCount; i++ {
			var artifactL *artifact.ListArtifactsOK
			count := int64(i)
			artifactL, err = getArtifactList(cs, ctx, projectName, v.Name, &defaultCountElements, &count)
			if err != nil {
				return
			}
			oneArtifact.countTags = len(artifactL.Payload)
			for _, a := range artifactL.Payload {
				oneArtifact.artifactSize += a.Size
			}
			artifactList = append(artifactList, oneArtifact)
		}
	}
	return
}

func humanArtifactSize(s int64) string {
	bf := float64(s)
	for _, unit := range []string{"", "Ki", "Mi", "Gi", "Ti", "Pi", "Ei", "Zi"} {
		if math.Abs(bf) < 1024.0 {
			return fmt.Sprintf("%3.1f%sB", bf, unit)
		}
		bf /= 1024.0
	}
	return fmt.Sprintf("%.1fYiB", bf)
}
