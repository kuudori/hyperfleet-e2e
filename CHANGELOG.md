# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- CONTRIBUTING.md with development setup and contribution guidelines
- CHANGELOG.md following Keep a Changelog format
- CLAUDE.md with AI agent instructions and validation checklist

### Changed
- Documentation structure to align with HyperFleet architecture standards

## [0.2.0] - 2024-XX-XX

### Added
- NodePool lifecycle tests
- Support for multiple test suites (cluster, nodepool)
- Label-based test filtering (tier0, tier1, tier2, slow, negative)
- JUnit report generation for CI integration
- Container image builds with multi-platform support

### Changed
- Improved test isolation with ephemeral resources
- Enhanced logging with structured output
- Updated OpenAPI client generation workflow

### Fixed
- Cleanup reliability for test resources
- Timeout handling in adapter status checks

## [0.1.0] - 2024-XX-XX

### Added
- Initial release of HyperFleet E2E testing framework
- Ginkgo-based test execution engine
- Cluster lifecycle tests
- OpenAPI-generated HyperFleet API client
- Configuration system with CLI flags, env vars, and config files
- Helper utilities for common test operations
- Payload template system with dynamic variable substitution
- Makefile with development, testing, and image build targets
- Comprehensive documentation (README, getting-started, architecture, development)
- CI/CD integration support

### Features
- Black-box testing approach
- Ephemeral test cluster creation
- Parallel test execution support
- Flexible test filtering by labels and patterns
- Structured logging (JSON and text formats)
- Automatic resource cleanup

[Unreleased]: https://github.com/openshift-hyperfleet/hyperfleet-e2e/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/openshift-hyperfleet/hyperfleet-e2e/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/openshift-hyperfleet/hyperfleet-e2e/releases/tag/v0.1.0
