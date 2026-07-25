package hf

import (
	"context"
	"reflect"
)

// scenario_test.go is the one list of things hf can be asked to do. The
// recorder drives it against the live hub to make fixtures, and the offline
// tests drive it against those fixtures. Keeping both on one table is what
// makes the coverage claim checkable: if a command is not here, no test touches
// it, and the shape goldens make that obvious.
//
// The references are pinned to entities that have been on the hub for years and
// exercise a specific shape: bert for a model with descendants and a paper,
// squad for a dataset the viewer can render, FLUX for a gated space with a
// runtime, and a Meta collection for items of mixed kinds.
const (
	fixModel      = "google-bert/bert-base-uncased"
	fixDataset    = "rajpurkar/squad"
	fixSpace      = "black-forest-labs/FLUX.1-dev"
	fixKernel     = "kernels-community/activation"
	fixUser       = "julien-c"
	fixOrg        = "google"
	fixNamespace  = "huggingface"
	fixCollection = "fdtn-ai/antares-6a5804c889e78b51c447c38a"
	fixPaper      = "1810.04805"
	fixBlog       = "gradio-mcp"
	fixPost       = "PeetPedro/340848718906662"
	fixChild      = "Jorgeutd/bert-base-uncased-finetuned-surveyclassification"
)

// runner executes one scenario and returns everything it emitted.
type runner func(ctx context.Context, c *Client) ([]any, error)

type scenario struct {
	Name string
	Deep bool // read the rendered page as well as the API
	Card bool // fetch the README and fold its front matter in
	Run  runner
}

// op adapts a typed operation to the scenario signature. The input is written
// out in full except for the client, which is filled in at run time because a
// recording run and a replay run use different ones.
func op[In any, T any](in In, fn func(context.Context, In, func(T) error) error) runner {
	return func(ctx context.Context, c *Client) ([]any, error) {
		v := in
		reflect.ValueOf(&v).Elem().FieldByName("C").Set(reflect.ValueOf(c))
		var out []any
		err := fn(ctx, v, func(rec T) error {
			out = append(out, rec)
			return nil
		})
		return out, err
	}
}

// call adapts a client method that is not an operation, for the few things the
// command surface reaches through cli rather than through kit.
func call[T any](fn func(ctx context.Context, c *Client) (T, error)) runner {
	return func(ctx context.Context, c *Client) ([]any, error) {
		v, err := fn(ctx, c)
		if err != nil {
			return nil, err
		}
		return []any{v}, nil
	}
}

// scenarios covers every registered operation at least once. The limits are
// small on purpose: a fixture set is only useful if a person can read it, and
// three rows prove a decoder as well as three hundred.
func scenarios() []scenario {
	return []scenario{
		// Reading one entity of each kind.
		{Name: "model", Run: op(refIn{Ref: fixModel}, getModel)},
		{Name: "dataset", Run: op(refIn{Ref: fixDataset}, getDataset)},
		{Name: "space", Run: op(refIn{Ref: fixSpace}, getSpace)},
		{Name: "kernel", Run: op(refIn{Ref: fixKernel}, getKernel)},
		{Name: "user", Run: op(nameIn{Name: fixUser}, getUser)},
		{Name: "org", Run: op(nameIn{Name: fixOrg}, getOrg)},
		{Name: "ns", Run: op(nameIn{Name: fixNamespace}, getNamespace)},
		{Name: "collection", Run: op(nameIn{Name: fixCollection}, getCollection)},
		{Name: "paper", Run: op(nameIn{Name: fixPaper}, getPaper)},
		{Name: "blog", Run: op(nameIn{Name: fixBlog}, getBlog)},
		{Name: "post", Run: op(nameIn{Name: fixPost}, getPost)},
		{Name: "discussion", Run: op(discussionIn{Repo: fixModel, Num: 1}, getDiscussion)},
		{Name: "get", Run: op(bareRefIn{Ref: fixModel}, getAny)},

		// The same reads with the page plane on, which is a different code path
		// and a different set of fields.
		{Name: "model-deep", Deep: true, Run: op(refIn{Ref: fixModel}, getModel)},
		{Name: "dataset-deep", Deep: true, Run: op(refIn{Ref: fixDataset}, getDataset)},
		{Name: "space-deep", Deep: true, Run: op(refIn{Ref: fixSpace}, getSpace)},
		{Name: "user-deep", Deep: true, Run: op(nameIn{Name: fixUser}, getUser)},
		{Name: "org-deep", Deep: true, Run: op(nameIn{Name: fixOrg}, getOrg)},
		{Name: "paper-deep", Deep: true, Run: op(nameIn{Name: fixPaper}, getPaper)},
		{Name: "collection-deep", Deep: true, Run: op(nameIn{Name: fixCollection}, getCollection)},
		{Name: "model-card", Card: true, Run: op(refIn{Ref: fixModel}, getModel)},

		// Lists and searches.
		{Name: "models", Run: op(listIn{Limit: 3}, listModels)},
		{Name: "models-filtered", Run: op(listIn{Limit: 3, Task: []string{"text-classification"}, Library: []string{"transformers"}}, listModels)},
		{Name: "models-full", Run: op(listIn{Limit: 3, Full: true, Sort: "downloads"}, listModels)},
		{Name: "datasets", Run: op(listIn{Limit: 3}, listDatasets)},
		{Name: "spaces", Run: op(listIn{Limit: 3}, listSpaces)},
		{Name: "kernels", Run: op(listIn{Limit: 3}, listKernels)},
		{Name: "collections", Run: op(collectionListIn{Limit: 2}, listCollections)},
		{Name: "collections-item", Run: op(collectionListIn{Limit: 2, Item: "models/" + fixModel}, listCollections)},
		{Name: "papers-daily", Run: op(paperListIn{Limit: 3, Daily: true}, listPapers)},
		{Name: "papers-search", Run: op(paperListIn{Limit: 3, Search: "attention is all you need"}, listPapers)},
		{Name: "posts", Run: op(limitIn{Limit: 3}, listPosts)},
		{Name: "blogs", Run: op(limitIn{Limit: 3}, listBlogs)},
		{Name: "discussions", Run: op(discussionListIn{Repo: fixModel, Limit: 3}, listDiscussions)},
		{Name: "search", Run: op(searchIn{Query: "bert", Limit: 5}, search)},
		{Name: "trending", Run: op(trendingIn{Limit: 3}, trending)},

		// Social relations.
		{Name: "followers", Run: op(nameLimitIn{Name: fixUser, Limit: 3}, followers)},
		{Name: "following", Run: op(nameLimitIn{Name: fixUser, Limit: 3}, following)},
		{Name: "likers", Run: op(repoLimitIn{Repo: fixModel, Limit: 3}, likers)},
		{Name: "likes", Run: op(nameLimitIn{Name: fixUser, Limit: 3}, likes)},
		{Name: "members", Run: op(nameLimitIn{Name: fixOrg, Limit: 3}, members)},
		{Name: "upvoters", Run: op(nameIn{Name: fixPaper}, upvoters)},

		// Repository contents.
		{Name: "card", Run: op(refIn{Ref: fixModel}, card)},
		{Name: "commits", Run: op(commitsIn{Repo: fixModel, Limit: 3}, commits)},
		{Name: "files", Run: op(repoLimitIn{Repo: fixModel}, files)},
		{Name: "refs", Run: op(bareRefIn{Ref: fixModel}, refs)},
		{Name: "tree", Run: op(treeIn{Repo: fixModel}, tree)},
		{Name: "tree-commits", Run: op(treeIn{Repo: fixModel, Commits: true, Limit: 5}, tree)},
		{Name: "paths", Run: op(pathsIn{Repo: fixModel, Path: []string{"config.json", "README.md"}}, paths)},
		{Name: "page", Run: op(bareRefIn{Ref: fixModel}, page)},
		{Name: "page-dataset", Run: op(bareRefIn{Ref: "datasets/" + fixDataset}, page)},

		// The dataset viewer.
		{Name: "valid", Run: op(datasetIn{Dataset: fixDataset}, valid)},
		{Name: "splits", Run: op(datasetIn{Dataset: fixDataset}, splits)},
		{Name: "size", Run: op(datasetIn{Dataset: fixDataset}, size)},
		{Name: "schema", Run: op(rowIn{Dataset: fixDataset}, schema)},
		{Name: "stats", Run: op(rowIn{Dataset: fixDataset}, stats)},
		{Name: "rows", Run: op(rowIn{Dataset: fixDataset, Limit: 2}, rows)},
		{Name: "head", Run: op(rowIn{Dataset: fixDataset, Limit: 2}, head)},
		{Name: "parquet", Run: op(datasetIn{Dataset: fixDataset}, parquet)},
		{Name: "dsearch", Run: op(dsearchIn{Dataset: fixDataset, Query: "beyonce", Limit: 2}, dsearch)},
		{Name: "filter", Run: op(filterIn{Dataset: fixDataset, Where: `"title"='Super_Bowl_50'`, Split: "validation", Limit: 2}, filter)},

		// The graph plane.
		{Name: "uri", Run: op(inputIn{Input: fixModel}, uriOf)},
		{Name: "url", Run: op(inputIn{Input: "https://huggingface.co/datasets/" + fixDataset}, urlOf)},
		{Name: "graph", Run: op(bareRefIn{Ref: fixModel}, graph)},
		{Name: "graph-dataset", Run: op(bareRefIn{Ref: fixDataset}, graph)},
		{Name: "edges", Run: op(bareRefIn{Ref: fixModel}, edges)},
		{Name: "children", Run: op(modelLimitIn{Model: fixModel, Limit: 3}, children)},
		{Name: "parents", Run: op(modelLimitIn{Model: fixChild}, parents)},
		{Name: "citations", Run: op(citationsIn{Paper: fixPaper, Limit: 3}, citations)},
		{Name: "crawl", Run: op(crawlIn{Ref: fixModel, Depth: 1, MaxNodes: 8, MaxReqs: 40}, crawl)},

		// The controlled vocabulary and the site's own indexes.
		{Name: "tags", Run: op(tagsIn{Type: "license"}, listTags)},
		{Name: "tasks", Run: op(clientIn{}, listTasks)},
		{Name: "sitemap", Run: op(sitemapIn{Kind: "models", Limit: 3}, listSitemap)},
		{Name: "dump", Run: op(dumpIn{Kind: "models", Limit: 3}, dump)},

		// Byte-producing reads. They belong here rather than with the cli
		// commands that wrap them, because what needs freezing is the fetch.
		{Name: "readme", Run: call(func(ctx context.Context, c *Client) (string, error) {
			return c.Readme(ctx, KindModel, fixModel, "")
		})},
		{Name: "file", Run: call(func(ctx context.Context, c *Client) (string, error) {
			resp, err := c.File(ctx, KindModel, fixModel, "", "config.json", false)
			if err != nil {
				return "", err
			}
			return string(resp.Body), nil
		})},
		{Name: "croissant", Run: call(func(ctx context.Context, c *Client) (string, error) {
			b, err := c.Croissant(ctx, fixDataset)
			return string(b), err
		})},
	}
}
