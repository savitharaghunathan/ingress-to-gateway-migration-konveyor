# orcapod-ingress-provisioner

A Go CLI tool that provisions standardized Ingress resources for tenant applications using nginx-ingress. Each tenant gets a consistent set of Ingress resources with the company's default security headers and TLS configuration.

This tool needs to be migrated from nginx-ingress to Gateway API before the March 2026 retirement deadline.

## Structure

`main.go` — provisions ingresses for the storefront (web frontend + API backend), with CRUD operations, ingress builder, IngressClass management, security headers, HSTS, and validation.

