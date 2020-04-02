## Problem

The NGINX documentation is broken up per module. Every module lists its directives and each directive lists the contexts where it can be used. The development of the adapter requires listing of directives broken up by contexts rather than by module. In other words, we need the answer to questions of the form:

	Given the context X, what directives are usable within it?

Once we have the listing of directives per context, we can look for those directives once the adapter enters certain context.

## Solution

The NGINX documentation is stored as XML files as part of the source code of the NGINX.org repository. The structured nature of the documentation means it can be traversed and restructured to fit our usecase, i.e. list directives per context rather than per module.

The `nginx-dirs.go` parses the XML files of the NGINX docs stored locally, collect the directives into context buckets, then output a collection of text files named after the context and containing the list of directives allowed within said context.

The text files serve as reference for contributors to know what directives are expected within a context. They serve as an exhaustive list and used as checklist items and an indicator of how complete is the coverage of the adapter.

## Usage 

1- Clone https://hg.nginx.org/nginx.org

2- Execute: `go run nginx-dirs.go <path to cloned repo in step 1>`