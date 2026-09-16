# KMJG Hub — User Experience Specification

## Document Status

Status: Draft  
Target: KMJG Hub v1

---

## Server Connection Experience

### First Launch

When KMJG Hub is launched for the first time, the user is presented with a welcome screen.

The primary action is:

    Connect to Server

The user enters the address of a self-hosted KMJG Hub Server.

Example:

    hub.example.com

After a successful connection, the user continues to authentication for that Server.

### Saved Servers

KMJG Hub remembers Servers that the user has previously connected to.

On future launches, the Client may display a Server selection screen such as:

    Your Servers

    KMJG Team
    hub.example.com
    [ Connect ]

    Internship Team
    kmjg.company.local
    [ Connect ]

    [ + Add Server ]

A single KMJG Hub Client may connect to multiple independent KMJG Hub Servers.

Accounts, Projects, Friends, Direct Messages, and other Server data remain separate between Servers.

Users may add another Server without removing their existing saved Servers.

---

## Authentication Experience

### Login

After connecting to a Server, users who are not currently authenticated are presented with the Server's login screen.

The screen should identify the Server the user is connecting to.

Example:

    Welcome to KMJG Team

    Username or Email
    [_____________________]

    Password
    [_____________________]

    [ Log In ]

    ---------- or ----------

    [ Continue with GitHub ]

    Don't have an account?
    [ Create Account ]

If the user already has a valid authenticated session for that Server, KMJG Hub should allow the user to continue without requiring them to log in again.

Authentication sessions are maintained separately for each saved Server.

### Registration

Users may create a local account on the selected KMJG Hub Server.

The registration experience should collect the minimum information required to create the account.

After successful registration, the user continues into that Server's KMJG Hub experience.

---

## Server Home

After authentication, the user is presented with the Home screen for the currently connected Server.

Projects are the primary focus of the Home screen.

Example:

    KMJG Team

    Your Projects

    KMJG Hub Development
    4 Members
    [ Open ]

    Game Center
    3 Members
    [ Open ]

    [ + Create Project ]
    [ Join Project ]

    Navigation:
    Projects | Friends | DMs | Notifications | Profile

KMJG Hub does not automatically open the user's most recently used Project.

The user selects the Project they want to enter each time they begin a Server session.

Server-level features such as Friends, Direct Messages, Notifications, and Profile remain accessible without entering a Project.

### Empty Project State

If the user does not belong to any Projects, the Home screen should provide clear actions to create or join one.

Example:

    You don't have any Projects yet.

    [ Create Project ]
    [ Join Project ]

---

## Project Workspace

When the user opens a Project, KMJG Hub displays the Project Workspace.

The Project Workspace uses a persistent sidebar for navigation between the Project's main collaboration features.

The primary navigation includes:

- Overview
- Chat
- Tasks
- Git
- Files
- Members
- Developer Tools

A control to return to the Server Home and Project selection is also available.

### Project Overview

Overview is the default screen when entering a Project.

It provides a quick summary of the Project without requiring the user to open multiple sections.

The Overview may display:

- Online members
- Member work status
- Current tasks
- Recent Git activity
- Recent Project activity
- Repository connection status

The Overview should prioritize information that helps a user quickly understand what the team is currently working on.

Example layout:

    KMJG Hub Development

    Overview

    Members Online
    3 / 4

    Current Tasks
    ...

    Recent Git Activity
    ...

    Recent Project Activity
    ...

---

## Members Experience

The Members section provides a quick view of who is currently available and what each Project member is working on.

Each member entry may display:

- Display name
- Online / Offline presence
- Work status
- Current branch
- Current task

Example:

    Members

    ● Korn
      Working · main
      Current Task: Design Project Workspace

    ● Meran
      Working · feature/auth
      Current Task: Authentication

    ○ Jai
      Offline

    ● Garflied
      Online · Not Working

Live work information should not be presented as current when a member is offline.

### Member Details

Selecting a Project member opens a compact member detail view.

When permitted by the member's privacy settings, the view may display:

- Presence
- Work status
- Current branch
- Current task

Quick actions include:

    [ Chat ]
    [ Send File ]
    [ View Branch ]
    [ View Current Task ]

Chat opens a Direct Message conversation with the selected member.

Send File starts the Direct File Transfer flow.

View Branch opens the relevant Git context when branch information is available.

View Current Task opens the selected member's Current Task when one is available and visible to the viewer.

---

## Project Chat Experience

Each Project has one primary Project Chat in KMJG Hub v1.

The Project Chat provides a shared conversation space for all authorized Project members.

The chat timeline may contain:

- Messages from Project members
- Project Chat file attachments
- Configured Git activity
- Relevant system activity

Git activity should be visually distinguishable from normal user messages.

Example:

    Project Chat

    Korn                                  14:32
    Push finished. You can pull now.

    ── Git Activity ───────────────────────────

    Korn pushed 3 commits → main

    • feat: add authentication
    • fix: server connection
    • docs: update README

    Meran                                 14:35
    Okay, I'll try it.

    [ + ] [ Message Project... ] [ Send ]

### Project Chat Attachments

Users may attach files to Project Chat messages.

Selecting the attachment action allows the user to choose a file before sending the message.

Project Chat attachments are stored by the Server according to the Project's storage and retention rules.

Other Project members download attachments only when they choose to do so.

### Direct Messages

Direct Messages are separate from Project Chat.

Private conversations between users must not appear in the Project Chat timeline.

KMJG Hub v1 does not use Discord-style multiple text channels within a Project.

---

## Task Experience

The Tasks section provides a simple Kanban-style view of Project work.

Tasks are organized into three columns:

- To Do
- In Progress
- Done

Example:

    Tasks                              [ + New Task ]

    To Do          In Progress             Done
    ──────         ───────────             ──────
    Login UI       Server API              README
    Meran          Korn                    Jai
                   ★ Current Task

    File Transfer
    Garflied

Users may create Tasks directly from the Tasks section.

Selecting a Task opens its detail view.

### Task Details

The Task detail view may display:

- Title
- Description
- Status
- Assignee
- Creator
- Due date
- Comments

When appropriate, the user may also:

    [ Set as Current Task ]

A user may have multiple assigned Tasks but only one Current Task at a time.

Selecting a To Do Task as the Current Task may automatically move it to In Progress.

Tasks are moved to Done manually.

---

## Git Experience

The Git section provides Project members with repository awareness and quick access to their existing development tools.

KMJG Hub v1 is not intended to replace a dedicated Git client, terminal, or IDE.

When a repository is connected, the Git section may display:

- Repository name
- Current local branch
- Local working tree status
- Number of commits ahead or behind
- Recent Project Git activity
- Relevant repository access information

Example:

    Git

    Repository
    itswskkk/KMJG-Hub

    Your Local Status

    Branch       main
    Changes      3 modified
    Ahead        2 commits
    Behind       0 commits

    Recent Activity

    Korn pushed 3 commits → main
    Meran pushed 1 commit → feature/auth

    [ Open Repository ]
    [ Open in Terminal ]
    [ Open in VS Code ]

Git-related actions should prioritize launching or integrating with the development tools the user already uses.

KMJG Hub v1 does not provide a full graphical Commit, Push, or Pull workflow.

Git operations remain under direct user control.

### No Repository Connected

A Project may be used without a connected Git repository.

When no repository is connected, the Git section should clearly communicate this state.

Example:

    No Repository Connected

    This Project is not connected to a Git repository yet.

    [ Connect Repository ]

The Connect Repository action is available only to users with the required Project permissions.

---

## Files Experience

The Files section provides access to files that have been shared with the Project through Project Chat.

Project Files and Direct File Transfers are separate experiences.

### Project Files

Files shared through Project Chat may be browsed from the Files section.

Each file entry may display:

- File name
- File size
- Sender
- Date shared
- Related Project Chat message

Example:

    Files

    architecture.pdf
    2.4 MB · Korn · Sep 16
    [ Download ] [ View Message ]

    api-example.json
    18 KB · Meran · Sep 15
    [ Download ] [ View Message ]

Files are not automatically downloaded to a member's device.

The user explicitly chooses when to download a Project file.

### Direct File Transfer

Direct File Transfer is used when one user sends a file directly to another user.

Before the recipient accepts the transfer, the request displays information such as:

    Korn wants to send you a file

    database-backup.zip
    24.8 MB

    [ Decline ] [ Accept ]

The file content must not be uploaded to Server storage before the recipient accepts the transfer.

After acceptance, KMJG Hub begins the transfer according to the supported transfer mechanism.

During transfer, the user should be able to see progress and cancel the transfer.

Example:

    database-backup.zip

    14.2 MB / 24.8 MB
    57%

    [ Cancel ]

If the transfer is interrupted, KMJG Hub should clearly report that the transfer did not complete.

Transfer resume is not required for v1.

Direct File Transfers should not appear as persistent Project Files unless the user separately shares the file through Project Chat.

---

## Developer Tools Experience

The Developer Tools section provides quick access to development tools using the currently selected Project's local directory as context.

Example:

    Developer Tools

    Project Directory
    /home/korn/KMJG-Hub

    Terminal        [ Open ]
    VS Code         [ Open ]
    Codex           [ Open ]
    Claude Code     [ Open ]
    OpenCode        [ Open ]

    [ Configure Tools ]

Selecting Open launches the corresponding external tool using the Project's local directory as its working context where supported.

KMJG Hub does not replace or embed these development tools.

### Tool Availability

KMJG Hub should clearly indicate when a configured tool is unavailable on the user's device.

Example:

    Terminal        [ Open ]
    VS Code         [ Open ]
    Claude Code     Not Installed

The Client should not silently fail when a tool cannot be launched.

Users may use Configure Tools to manage available tool integrations and local executable configuration where necessary.

---

## Friends and Direct Messages Experience

Friends and Direct Messages are Server-level features and remain accessible without entering a Project.

### Friends

The Friends screen displays the user's friends on the current KMJG Hub Server.

Example:

    Friends

    ● Meran       Online       [ Message ]
    ● Jai         Working      [ Message ]
    ○ Garflied    Offline      [ Message ]

    [ + Add Friend ]

    Pending Requests (1)

    Somchai
    [ Accept ] [ Decline ]

Users may send Friend Requests to other users on the same Server.

Friend Requests require the recipient to Accept or Decline before a friendship is created.

Leaving a shared Project does not remove an existing friendship.

### Direct Messages

The Direct Messages screen provides access to the user's private conversations on the current Server.

Example:

    Direct Messages

    Meran       Okay, I'll pull first.       2m
    Jai         I sent the file.             1h
    Garflied    👍                            Yesterday

Selecting a conversation opens the Direct Message view.

Users may start Direct Messages with:

- Friends
- Members of a shared Project

Users who are neither friends nor members of a shared Project cannot start a Direct Message and may instead send a Friend Request.

### Direct Message Actions

A Direct Message conversation may provide actions such as:

    [ Send File ]

Send File starts the Direct File Transfer experience.

### Blocking

Users may block another user.

Blocking:

- Prevents Direct Messages between the users
- Prevents Friend Requests between the users
- Removes an existing friendship
- Does not change Project membership or Project permissions

When appropriate, KMJG Hub may explicitly inform the blocked user that they have been blocked.

Unblocking a user does not automatically restore the previous friendship.

---

## Notifications Experience

Notifications are available at the Server level and may include events from multiple Projects.

Users can access notifications through a notification indicator such as:

    🔔 Notifications

The notification view may contain:

    Meran assigned you a task
    KMJG Hub Development · 2m

    Jai wants to send you a file
    5m
    [ Accept ] [ Decline ]

    Korn pushed to main
    Game Center · 20m

    You were invited to Project X
    1h
    [ Accept ] [ Decline ]

Selecting a notification should navigate the user to the relevant context when possible, such as:

- Task
- Direct Message
- Project
- Git activity
- Project invitation
- File transfer request

KMJG Hub should prioritize notifications that require attention or are directly relevant to the user.

Routine Project Chat messages should not generate notifications by default.

---

## Profile Experience

The Profile screen represents the user's identity on the current KMJG Hub Server.

The profile may display:

- Avatar
- Display name
- Username
- Bio
- Presence
- Work status
- Current Project
- Current Task
- Current Branch
- Connected Git provider
- Repositories the user chooses to expose

Example:

    Profile

    [ Avatar ]   Korn
                 @korn

    Bio
    Computer Engineering student

    Presence          ● Online
    Work Status       Working
    Current Project   KMJG Hub Development
    Current Task      UX Design
    Current Branch    main

    Connected Git
    GitHub · itswskkk

    Repositories
    KMJG-Hub

    [ Edit Profile ]
    [ Privacy Settings ]

### Profile Privacy

Users control the visibility of supported profile fields individually.

Example:

    Privacy Settings

    Current Project   [ Friends & Project Members ]
    Current Task      [ Project Members ]
    Current Branch    [ Project Members ]
    Repositories      [ Everyone on Server ]

Available audiences may include:

- Everyone on the same Server
- Friends
- Project Members, where applicable
- Friends & Project Members
- Nobody

A viewer should only see profile information permitted by the owner's privacy settings.

### Private Repository Visibility

Before a user exposes a private repository on their profile, KMJG Hub must clearly warn that the repository name and selected metadata may become visible to the chosen audience.

Displaying a private repository on a profile does not:

- Make the repository public
- Grant repository access
- Expose repository files
- Expose branches or commits automatically
- Override Git provider permissions

### Offline Profiles

When a user is offline, KMJG Hub should not present Work Status, Current Task, or Current Branch as current live information.

Static profile information remains visible according to the user's privacy settings.

---

## Project Creation Experience

Users may create a new Project from the Server Home.

The creation flow should request only the information necessary to start the Project.

Example:

    Create Project

    Project Name
    [________________________]

    Description (Optional)
    [________________________]

    Repository

    ( ) Create New GitHub Repository
    ( ) Connect Existing Repository
    (•) Set Up Later

                         [ Create Project ]

The user who creates the Project automatically becomes its initial Owner.

After successful creation, KMJG Hub opens the new Project Workspace.

### Repository Setup

If Create New GitHub Repository is selected, KMJG Hub guides the user through the required GitHub repository creation options.

If Connect Existing Repository is selected, the user may:

- Select a repository from their connected GitHub account
- Enter an existing Git repository URL

If Set Up Later is selected, the Project is created without a repository.

The repository may be connected later by an authorized Project user.

---

## Project Join Experience

Users may join a Project from the Server Home using a valid Project invite link or invite code.

Example:

    Join Project

    Invite Link or Code
    [____________________________]

    [ Join Project ]

When the user intentionally submits a valid invite link or code, KMJG Hub joins the Project immediately without requiring an additional approval step.

After joining successfully, the user is added as a Member and the Project Workspace opens.

### Invalid Invitations

If an invitation cannot be used, KMJG Hub should clearly explain the reason where possible.

Examples include:

- Invalid invite
- Expired invite
- Invite has reached its maximum number of uses
- Invite has been revoked
- User is already a Project member

### Direct Project Invitations

Direct invitations from another user are different from invite links and codes.

A Direct Project Invitation requires:

    [ Decline ] [ Accept ]

Accepting the invitation adds the user to the Project as a Member.

Declining the invitation does not add the user to the Project.

---

## Project Settings Experience

Project Settings are accessed through a settings control near the Project name rather than occupying a primary Workspace navigation item.

Example:

    KMJG Hub Development  ⚙

Project Settings may be organized into:

- General
- Members & Roles
- Invitations
- Git Integration
- Notifications
- Danger Zone

Available settings and actions depend on the user's Project role and permissions.

Users should not be shown destructive or administrative actions that they are not permitted to perform.

### General

General settings contain basic Project information and configuration.

### Members & Roles

Authorized users may manage Project members and permitted role changes.

### Invitations

Authorized users may:

- Create invite links or codes
- Configure expiration
- Configure maximum uses
- Revoke active invites
- View or cancel pending Direct Invitations

### Git Integration

Authorized users may manage the Project's repository connection and supported Git integration settings.

### Notifications

The Owner may configure Git push notification behavior, including relevant members and branches.

### Danger Zone

Destructive or ownership-related actions are separated into a clearly identifiable Danger Zone.

Depending on permissions, these may include:

- Leave Project
- Transfer Ownership
- Delete Project

Destructive actions should require explicit confirmation before execution.

---

## Project Deletion and Recovery Experience

Only the Project Owner may delete a Project through the KMJG Hub Client.

Project deletion is a destructive action and requires explicit confirmation.

Example:

    Delete KMJG Hub Development?

    The Project will become unavailable immediately
    and can be restored for 30 days.

    The connected Git repository will NOT be deleted.

    Type the Project name to confirm:
    [________________________]

    [ Cancel ]  [ Delete Project ]

After deletion, the Project is removed from normal Project access and the user returns to the Server Home.

Deleting a KMJG Hub Project does not delete its connected external Git repository.

### Recently Deleted Projects

The original Project Owner may access deleted Projects through a recovery area.

Example:

    Recently Deleted Projects

    KMJG Hub Development
    23 days remaining

    [ Restore ]

The interface should clearly display the remaining recovery period.

Restoring the Project returns it to normal Project access.

After the 30-day retention period expires, the Project is no longer recoverable through the normal Client recovery experience.

Server backup retention remains separate from this recovery period.

---

## Server Switching Experience

KMJG Hub allows users to switch between previously connected Servers without requiring them to log out of the current Server first.

A Server switcher should be accessible from the Server-level interface.

Example:

    [ KMJG Team ▼ ]

    ✓ KMJG Team
      Company Server

    ----------------
    + Add Server
      Manage Servers

Selecting another Server changes the active Server context.

Server-level data must remain separate between Servers, including:

- Projects
- Friends
- Direct Messages
- Notifications
- Profile
- Authentication session

If a valid authenticated session exists for the selected Server, the user may continue without logging in again.

If authentication is required, KMJG Hub displays that Server's login screen.

### Manage Servers

Users may manage Servers previously added to the Client.

The Server management experience may allow users to:

- View saved Servers
- Add another Server
- Remove a Server from the Client
- Reconnect to an unavailable Server

Removing a saved Server from the Client does not delete the Server itself or any server-side account or Project data.

---

## Offline Experience

KMJG Hub should remain usable in a limited state when connectivity to the active Server is lost.

The user should not be automatically removed from the current Project Workspace.

A clear connection status should be displayed.

Example:

    ⚠ Offline — Showing cached information
      Reconnecting...

While offline, users may continue viewing appropriate locally available information such as:

- Previously loaded Project information
- Previous conversations
- Downloaded files
- Last known Git information
- Other available cached information

Features that require Server connectivity should be unavailable while offline.

Unavailable actions should clearly explain that connectivity is required rather than silently failing.

When the connection returns, KMJG Hub should indicate that synchronization is occurring.

Example:

    ✓ Connected — Syncing...

After synchronization completes, the Client returns to its normal connected state.

Live information such as Presence, Work Status, Current Task, and Current Branch must not be presented as current while the Server is unreachable.

---

## Leaving a Project Experience

Members and Admins may leave a Project from the Project Settings Danger Zone.

Before leaving, KMJG Hub should clearly explain the effect of the action.

Example:

    Leave KMJG Hub Development?

    You will lose access to this Project workspace.

    ☑ Also remove my repository access
      (where supported by the Git provider)

    [ Cancel ]  [ Leave Project ]

Removing external repository access is optional.

If repository access removal fails, the user must still be allowed to leave the KMJG Hub Project.

KMJG Hub should clearly report the partial result.

Example:

    You left the Project successfully.

    Your repository access could not be removed
    and may still remain with the Git provider.

### Owner Leaving

A Project Owner cannot leave while they still own the Project.

If the Owner attempts to leave, KMJG Hub should direct them to transfer ownership to another Project member first.

After ownership has been transferred, the previous Owner may leave normally.

---

## Removing a Project Member Experience

Authorized Project users may remove members according to their role permissions.

Before removing a member, KMJG Hub should request confirmation.

Example:

    Remove Meran from this Project?

    ☑ Also remove repository access
      (where permitted by the Git provider)

    [ Cancel ]  [ Remove Member ]

Removing repository access is optional and remains controlled by the external Git provider.

If repository access removal fails, the member should still be removed from the KMJG Hub Project.

KMJG Hub must clearly communicate the partial result.

Example:

    Meran was removed from the Project.

    Repository access could not be removed
    and may still remain with the Git provider.

The interface must respect Project role permissions.

For example, an Admin may remove a Member but cannot remove the Owner or another Admin.

---

## Repository Access Experience

KMJG Hub should clearly distinguish Project membership from repository access.

A Project member may belong to a KMJG Hub Project without having access to its connected repository.

Example:

    Repository Access

    Meran
    No repository access

    [ Invite to Repository ]

### Repository Invitation

When an authorized user has sufficient permissions with the Git provider, they may invite a Project member to the connected repository through KMJG Hub.

Example:

    Invite Meran to Repository

    Permission
    [ Select Permission ▼ ]

    [ Cancel ] [ Send Invitation ]

The available permission levels should be provided by or mapped appropriately to the connected Git provider.

KMJG Hub must not assume that every Git provider uses the same permission model.

KMJG Hub must not bypass external repository permissions.

If the invitation cannot be sent, the Client should clearly explain the failure and leave the user's KMJG Project membership unchanged.
