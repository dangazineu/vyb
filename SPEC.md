# vyb CLI Specification

This document describes the design and functionality of the
"vyb" command-line application, based on the existing codebase. If new
requirements emerge or unimplemented features are discovered, they are
prefixed with `[TODO]:`.

## Programming Language

This application is written in Go (version 1.24 or higher).

## Frameworks and Libraries

- [Cobra](https://github.com/spf13/cobra) for CLI creation.
- [Mustache](https://github.com/cbroglie/mustache) for template
  processing.
- [YAML.v3](https://pkg.go.dev/gopkg.in/yaml.v3) for reading and
  writing metadata.
- [OpenAI API](https://openai.com/) integration implemented over REST.
- [TODO]: Add support for other LLM backend vendors.


## Application Type

- Command-Line Interface (CLI)

## Architecture and Design

- The application is organized into multiple Go packages:
  - `cmd`: Entry point for commands and subcommands using Cobra.
  - `llm`: Handles interactions with the OpenAI API, including
    request/response payloads.
  - `workspace`: Manages file/directory patterns, metadata creation,
    and project structure.
- The root command (`vyb`) registers subcommands (e.g., `init`,
  `remove`, etc.) and dynamically loads additional commands (such as
  `refine`, `code`, `inferspec`) using embedded `.vyb` config files.
- User-defined commands are loaded from `.vyb` files stored under
  `$VYB_HOME/cmd/`.
- [TODO]: Also load user-defined commands from the `.vyb/` folder in
  the application root.
- Project metadata is stored under a `.vyb` directory in the project's
  root, keeping track of configuration details in `metadata.yaml`.
- The code uses a structured output schema to parse JSON responses
  from OpenAI.

## Dependencies and Integrations

- Integrates with the OpenAI API to process instructions and handle
  generated proposals.
- Requires a valid `OPENAI_API_KEY` environment variable for
  authentication.
- Uses the standard Go tooling for builds and tests.

## User Interface

- Text-based CLI interface with subcommands. Examples include:
  - `vyb init` for creating `.vyb/metadata.yaml`.
  - `vyb remove` for deleting all metadata.
  - `vyb refine` or `vyb inferspec` for updating specification files.
  - `vyb code` and `vyb document` for code generation and doc updates.

## Performance and Scalability Requirements

- The CLI is designed for local usage. No special scalability measures
  are currently in place.
- [TODO]: Explore concurrency or caching mechanisms for large
  workspaces.

## Security Considerations

- The application reads `OPENAI_API_KEY` from the environment.
- No extra encryption or advanced security measures currently.
- [TODO]: Evaluate secure storage or encryption for API keys.

## Deployment Environment

- Project can be built and run locally with `go build -o vyb`.
- The CLI runs on any environment supporting Go 1.24+.

## Operational Aspects

- Commands log errors to stdout and exit with non-zero status on
  failure.
- [TODO]: Integrate deeper logging or monitoring if needed.

## Testing and Quality Assurance

- Uses Go testing framework (`go test`) in each directory.
- Contains unit tests for file matching, metadata handling, and
  OpenAI payloads.
- [TODO]: Evaluate coverage levels and add integration tests.

## Documentation and Maintenance

- The repository includes `README.md` files at various levels.
- This `SPEC.md` documents the established functionality.
- [TODO]: Update or extend as new features are introduced.
