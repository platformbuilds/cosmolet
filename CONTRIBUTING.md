# Contributing to Cosmolet

Thank you for your interest in contributing to Cosmolet! This document provides guidelines and information for contributors.

## 🤝 Code of Conduct

This project adheres to a Code of Conduct. By participating, you are expected to uphold this code.

## 🎯 How to Contribute

### Reporting Issues

- Use the GitHub issue tracker to report bugs or request features
- Search existing issues before creating a new one
- Include as much relevant information as possible
- Use the provided issue templates when available

### Submitting Changes

1. **Fork the repository**
   ```bash
   git clone https://github.com/cosmolet/cosmolet.git
   cd cosmolet
   git remote add upstream https://github.com/cosmolet/cosmolet.git
   ```

2. **Create a feature branch**
   ```bash
   git checkout -b feature/your-feature-name
   ```

3. **Make your changes**
   - Follow the coding standards outlined below
   - Write tests for new functionality
   - Update documentation as needed

4. **Test your changes**
   ```bash
   make test
   ```

5. **Commit your changes**
   ```bash
   git add .
   git commit -m "Add feature: your descriptive commit message"
   ```

6. **Push to your fork**
   ```bash
   git push origin feature/your-feature-name
   ```

7. **Create a Pull Request**
   - Use the PR template
   - Link any related issues
   - Ensure CI passes

## 🔧 Development Setup

### Prerequisites

- Go 1.21+
- Docker
- kubectl
- make

### Local Development

1. **Setup development environment**
   ```bash
   make dev-setup
   ```

2. **Install dependencies**
   ```bash
   go mod download
   ```

3. **Run locally**
   ```bash
   make run
   ```

### Testing

- **Unit tests**: `make test`
- **Linting**: `make lint`
- **All checks**: `make check`

## 📝 Coding Standards

### Go Code Style

- Follow standard Go formatting (`gofmt`)
- Use meaningful variable and function names
- Write comprehensive comments for exported functions
- Handle errors appropriately

### Commit Messages

Follow the [Conventional Commits](https://www.conventionalcommits.org/) specification:

```
type(scope): description

[optional body]

[optional footer]
```

Examples:
```
feat(bgp): add support for BGP communities
fix(controller): handle service endpoint updates correctly
docs: update installation instructions
```

## 📄 License

By contributing to Cosmolet, you agree that your contributions will be licensed under the [GNU Affero General Public License v3.0](LICENSE).

---

Thank you for contributing to Cosmolet! 🚀
