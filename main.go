package main

import (
	"bytes"
	"context"
	"fmt"
	"github.com/shurcooL/githubv4"
	"github.com/yuin/goldmark"
	"golang.org/x/oauth2"
	"os"
	"regexp"
	"strings"
	"text/template"
	"unicode"
)

// Configuration
const (
	readmeTmplFile = `README.tmpl`
	indexTmplFile  = `index.tmpl`
	readme         = `README.md`
	index          = `index.html`
	username       = `gududege`
	repositoryName = `Starred-Repository-Monitor`
)

// Setting
const (
	envGithubTokenName          = `USER_GITHUB_TOKEN`
	errTokenNotFoundDescription = `$USER_GITHUB_TOKEN environment variable not set.`
	patternContentSymbol        = `[_|-]`
	spaceChar                   = " "
	emptyChar                   = ""
	step1Description            = "Step1 - Check congfiguration: "
	step2Description            = "Step2 - Get user starred repositories information via Github api: "
	step3Description            = "Step3 - Render README template file with data: "
	step4Description            = "Step4 - Render index.html template file with README content: "
	step5Description            = "Step5 - Write output: "
)

// Return Code
const (
	OK = iota
	ErrCodeRegexFault
	ErrCodeNoTokenGiven
	ErrCodeFileNotFound
	ErrCodeGithubQuery
	ErrCodeRenderTemplate
	ErrCodeConvertMarkdown
	ErrCodeWriteOutput
)

type RepositoryInfo struct {
	Name           string
	NameWithOwner  string
	Description    string
	Url            string
	StargazerCount int
	ForkCount      int
	UpdatedAt      string
	CreatedAt      string
	PushedAt       string
	IsArchived     bool
	Languages      []string
}

func executeTemplateToStr(tmpl string, data any) (string, error) {
	t := template.New("local_template")
	parsedTmpl, err := t.Parse(tmpl)
	if err != nil {
		return "", err
	}
	buf := new(bytes.Buffer)
	err = parsedTmpl.Execute(buf, data)
	return buf.String(), err
}

func convertMarkdownToHTML(markdown string) (string, error) {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(markdown), &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func getUserStaredRepositories(githubToken string) ([]RepositoryInfo, error) {
	stars := make([]RepositoryInfo, 0)
	src := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubToken},
	)
	httpClient := oauth2.NewClient(context.Background(), src)
	client := githubv4.NewClient(httpClient)
	var query struct {
		Viewer struct {
			StarredRepositories struct {
				IsOverLimit bool
				TotalCount  int
				Edges       []struct {
					StarredAt string
					Cursor    string
					Node      struct {
						Name           string
						NameWithOwner  string
						Description    string
						Url            string
						StargazerCount int
						ForkCount      int
						UpdatedAt      string
						CreatedAt      string
						PushedAt       string
						IsArchived     bool
						Languages      struct {
							TotalCount int
							Nodes      []struct {
								Name string
							}
						} `graphql:"languages(first: 3)"`
					}
				}
			} `graphql:"starredRepositories(first: $count, after: $cursor, orderBy: {field: STARRED_AT, direction: DESC})"`
		}
	}
	variables := map[string]interface{}{
		"count":  githubv4.Int(100),
		"cursor": githubv4.String(""),
	}
	// Initial value
	totalRepositories := 100
	totalFound := 0
	currentFound := 0
	cursor := ""
	for totalFound < totalRepositories {
		err := client.Query(context.Background(), &query, variables)
		if err != nil {
			return stars, err
		}
		// For next loop
		totalRepositories = query.Viewer.StarredRepositories.TotalCount
		currentFound = len(query.Viewer.StarredRepositories.Edges)
		totalFound += currentFound
		cursor = query.Viewer.StarredRepositories.Edges[currentFound-1].Cursor
		variables["count"] = min(githubv4.Int(totalRepositories-currentFound), githubv4.Int(100))
		variables["cursor"] = githubv4.String(cursor)
		// Storage repository info
		for _, edge := range query.Viewer.StarredRepositories.Edges {
			// Remove emoji symbol in description.
			edge.Node.Description = strings.TrimFunc(edge.Node.Description, func(r rune) bool {
				return !unicode.IsLetter(r) && !unicode.IsNumber(r) //&& !unicode.IsSpace(r) && !unicode.IsPunct(r)
			})
			languages := make([]string, 0)
			for _, node := range edge.Node.Languages.Nodes {
				languages = append(languages, node.Name)
			}
			stars = append(stars, RepositoryInfo{
				Name:           edge.Node.Name,
				NameWithOwner:  edge.Node.NameWithOwner,
				Description:    edge.Node.Description,
				Url:            edge.Node.Url,
				StargazerCount: edge.Node.StargazerCount,
				ForkCount:      edge.Node.ForkCount,
				UpdatedAt:      edge.Node.UpdatedAt,
				CreatedAt:      edge.Node.CreatedAt,
				PushedAt:       edge.Node.PushedAt,
				IsArchived:     edge.Node.IsArchived,
				Languages:      languages,
			})
		}
	}

	return stars, nil
}

func main() {
	// Step1: Check configuration
	fmt.Print(step1Description)
	token := os.Getenv(envGithubTokenName)
	if token == "" {
		fmt.Println(errTokenNotFoundDescription)
		os.Exit(ErrCodeNoTokenGiven)
		return
	}

	regex, err := regexp.Compile(patternContentSymbol)
	if err != nil {
		fmt.Println(err)
		os.Exit(ErrCodeRegexFault)
		return
	}
	title := strings.Trim(regex.ReplaceAllString(repositoryName, spaceChar), spaceChar)

	readmeTmplBytes, err := os.ReadFile(readmeTmplFile)
	if err != nil {
		fmt.Println(err)
		os.Exit(ErrCodeFileNotFound)
		return
	}

	indexTmplBytes, err := os.ReadFile(indexTmplFile)
	if err != nil {
		fmt.Println(err)
		os.Exit(ErrCodeFileNotFound)
		return
	}
	fmt.Println("OK.")

	// Step2: Query stars
	fmt.Print(step2Description)
	repos, err := getUserStaredRepositories(token)
	if err != nil {
		fmt.Println(err)
		os.Exit(ErrCodeGithubQuery)
		return
	}
	fmt.Println("OK.")

	// Step3: Render README template
	fmt.Print(step3Description)
	readmeContent, err := executeTemplateToStr(string(readmeTmplBytes), map[string]interface{}{
		`Title`:            title,
		`RepositoryName`:   repositoryName,
		`UserName`:         username,
		`RepositoriesInfo`: repos,
	})
	if err != nil {
		fmt.Println(err)
		os.Exit(ErrCodeRenderTemplate)
		return
	}
	fmt.Println("OK.")

	// Step4: Render html template
	fmt.Print(step4Description)
	indexStr, err := convertMarkdownToHTML(readmeContent)
	if err != nil {
		fmt.Println(err)
		os.Exit(ErrCodeConvertMarkdown)
		return
	}
	indexContent, err := executeTemplateToStr(string(indexTmplBytes), map[string]interface{}{
		`Title`:             title,
		`RepositoryName`:    repositoryName,
		`ReadmeContent`:     indexStr,
		`RepositoriesCount`: len(repos),
	})
	if err != nil {
		fmt.Println(err)
		os.Exit(ErrCodeRenderTemplate)
		return
	}
	fmt.Println("OK.")

	// Step5: Write to README
	fmt.Print(step5Description)
	if err := os.WriteFile(readme, []byte(readmeContent), 0644); err != nil {
		fmt.Println(err)
		os.Exit(ErrCodeWriteOutput)
		return
	}
	if err := os.WriteFile(index, []byte(indexContent), 0644); err != nil {
		fmt.Println(err)
		os.Exit(ErrCodeWriteOutput)
		return
	}
	fmt.Println("OK.")
	fmt.Println("Finished!")
	os.Exit(OK)
}
