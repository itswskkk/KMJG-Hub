# KMJG Hub — Vision

## What is KMJG Hub?

KMJG Hub is a cross-platform developer collaboration hub designed for small software development teams.

The goal of KMJG Hub is to bring the collaboration tools developers use every day into a single project-focused workspace, reducing the need to constantly switch between communication, file-sharing, Git, and project management applications.

KMJG Hub is not intended to replace development tools such as VS Code, terminals, Git, or other developer tools. Instead, it acts as a central hub that connects the team, the project, and the tools developers already use.

---

## The Problem

Software development teams often rely on several separate applications to collaborate.

A typical workflow may involve:

- LINE or Discord for communication
- Google Drive for sharing files
- GitHub for source control and project activity
- A browser for repository management
- Terminals and IDEs for development
- Additional tools for tasks and project coordination

Switching between these applications repeatedly creates unnecessary friction and breaks development flow.

KMJG Hub was inspired by this problem during real software development and internship collaboration.

---

## The Vision

KMJG Hub aims to provide one central workspace where a development team can quickly understand:

- Who is currently online
- What teammates are working on
- Which Git branches are being used
- Recent Git activity
- Project conversations
- Shared files
- Project tasks
- Important project notifications

Instead of asking:

> "Which application do I need to open?"

The goal is for the developer to start from:

> "Open the project in KMJG Hub."

From there, KMJG Hub connects them to the people, information, and tools associated with that project.

---

## Project-First Design

KMJG Hub follows a project-first design.

When the application starts, the user selects a project and enters its workspace.

Each project acts as the central context for collaboration.

For the initial version:

- One KMJG Hub Project is associated with one Git repository.
- A user may belong to multiple projects.
- Each project has its own members, conversations, files, Git activity, tasks, and notifications.

This makes the project — rather than chat channels or servers — the center of the experience.

---

## Git as a First-Class Part of the Workspace

Git is a core part of KMJG Hub.

The application should understand the Git context of a project and make useful repository information visible to the team.

Examples include:

- Repository information
- Current branches
- Recent commits
- Push activity
- Modified files where appropriate
- Team Git activity notifications

When a team member pushes changes, other project members should be able to see that activity without constantly checking GitHub manually.

However, KMJG Hub should keep developers in control of Git operations.

The goal is not to let AI automatically commit or push code on behalf of the team.

---

## Communication

Communication is part of the workspace, but KMJG Hub is not designed to be a Discord replacement.

Projects may provide collaboration spaces such as:

- General discussion
- Announcements
- Git activity
- Files
- Tasks

Users may also communicate directly with other members through private messages.

The purpose of communication inside KMJG Hub is to keep project discussions close to the development context.

---

## File Sharing

KMJG Hub should make file sharing between developers simple.

Instead of uploading a file to another service, creating a share link, configuring permissions, and sending that link to a teammate, users should be able to send files directly from the project workspace.

File transfers should require the receiving user to explicitly:

- Accept
- Decline

Files should never be downloaded automatically without the receiver's permission.

The long-term goal is to support file transfers without artificial file-size limits imposed by communication platforms, while keeping the system practical to self-host.

---

## Developer Tools

KMJG Hub should integrate with developer tools rather than replace them.

Examples may include:

- Terminal
- VS Code
- Codex
- Claude Code
- OpenCode
- Other command-line development tools

KMJG Hub may launch these tools using the currently selected project's directory as context.

For example, a developer could open a terminal or supported coding tool directly in the project's working directory.

The terminal remains an important part of the development workflow.

---

## Cross-Platform

KMJG Hub is designed as a cross-platform application.

The initial desktop targets are:

- Linux
- Windows

The architecture should avoid unnecessary platform-specific assumptions so that macOS support can be added in the future.

A lightweight web dashboard may also be considered in a later version for viewing notifications, Git activity, and project status.

The desktop application remains the primary KMJG Hub experience.

---

## Self-Hosting

KMJG Hub should be self-hostable.

Small teams should be able to operate their own KMJG Hub infrastructure without depending entirely on a commercial hosted service.

The project should aim to keep infrastructure requirements lightweight and, where practical, allow very small teams to operate KMJG Hub at little or no cost.

---

## Offline Support

KMJG Hub should provide partial offline functionality.

When disconnected from the server, users should still be able to access appropriate cached information such as:

- Previously loaded project information
- Previous conversations
- Downloaded files
- Last known Git or member status

Real-time functionality such as sending messages, receiving notifications, or live presence requires connectivity.

When connectivity returns, the client should synchronize with the server where appropriate.

---

## Target Users

KMJG Hub is primarily designed for small software development teams.

Examples include:

- Student development teams
- Internship teams
- Small software teams
- Small open-source teams

The initial real-world use case is a four-person development team.

This gives the project a practical environment where features can be tested against real collaboration problems instead of being designed only as theoretical features.

---

## What KMJG Hub Is Not

KMJG Hub is not intended to become:

- An IDE
- A terminal replacement
- A Discord clone
- A GitHub Desktop clone
- A cloud storage replacement
- An AI coding assistant
- A remote desktop application

Instead, KMJG Hub connects collaboration and development context around a project.

---

## Product Principle

Every major feature should answer one question:

> Does this reduce unnecessary context switching or make software team collaboration easier?

If a feature does not meaningfully help with that goal, it probably does not belong in the core KMJG Hub experience.

---

## Long-Term Goal

The long-term goal of KMJG Hub is to become a lightweight, open, and self-hostable collaboration workspace built specifically around the way software development teams work.

Developers should remain free to use their preferred editors, terminals, Git workflows, and coding tools.

KMJG Hub simply provides the shared workspace that connects them.
