# KMJG Hub — Product Requirements Document

## Document Status

Status: Draft  
Target: KMJG Hub v1

---

## Product Overview

KMJG Hub is a cross-platform developer collaboration workspace for small software development teams.

Its primary goal is to reduce unnecessary context switching by bringing project communication, team presence, Git awareness, file sharing, and project collaboration into one project-focused application.

KMJG Hub is not intended to replace development tools such as terminals, IDEs, Git, or other developer tools.

Instead, KMJG Hub acts as a central workspace that connects developers, projects, collaboration features, and the development tools they already use.

---

## v1 Goals

KMJG Hub v1 should allow a small software development team to:

- Create or join a project workspace
- Associate a project with a Git repository
- See project members and their online status
- See basic work context such as current branch and current task
- Communicate through project chat
- Communicate through direct messages
- Send and receive files with explicit Accept / Decline
- View relevant Git activity
- Receive notifications when team members push changes
- Launch supported developer tools using the project's local directory
- Continue viewing appropriate cached information while offline

---

## v1 Platforms

Initial desktop support:

- Linux
- Windows

Future consideration:

- macOS
- Lightweight web dashboard

The desktop application is the primary KMJG Hub experience.

---

## Core Product Model

KMJG Hub follows a project-first design.

For KMJG Hub v1:

- One Project may be associated with a maximum of one Git repository.
- A Project may exist without a connected Git repository and connect one later.
- One user may belong to multiple Projects.
- Users select a Project when opening KMJG Hub.
- Each Project provides its own Workspace.
- The Project is the primary collaboration context, not a Discord-style server.
- Project data is accessible only to authorized project members.

A Project Workspace may contain:

- Members
- Presence and work status
- Project conversations
- Direct access to collaboration features
- Files
- Git activity
- Tasks
- Notifications

### Project Communication Model

Each Project has one primary Project Chat in KMJG Hub v1.

The Project Chat is the shared conversation space for authorized Project members.

The Project Chat may contain:

- Messages from Project members
- Project Chat file attachments
- Configured Git activity
- Relevant system activity

KMJG Hub v1 does not provide Discord-style multiple text channels within a Project.

Direct Messages are separate Server-level conversations and do not belong to the Project Chat.

---

## Deployment Model

KMJG Hub is an open-source, self-hosted application.

Users can download and deploy their own KMJG Hub Server and connect KMJG Hub Clients to that server.

A KMJG Hub deployment consists of:

- KMJG Hub Client
- KMJG Hub Server
- A domain or server address configured by the Server Administrator
- Server-side storage
- Server-side database
- Deployment-specific configuration

The KMJG Hub source code and official releases may be publicly available, while data belonging to individual deployments remains private to those deployments.

Each self-hosted KMJG Hub Server operates independently and maintains its own:

- User accounts
- Projects
- Project members
- Conversations
- Direct messages
- Files
- Tasks
- Git integration data
- Application configuration

Private deployment data must never be stored in the public KMJG Hub source repository.

---

## Open Source and Privacy

KMJG Hub follows a simple principle:

> The software is open. The user's data is private.

Open sourcing KMJG Hub means that the application's source code can be inspected, modified, built, redistributed, and self-hosted subject to the project's open-source license.

It does not mean that conversations, files, credentials, project data, or other private information from a KMJG Hub deployment are publicly accessible.

Sensitive deployment-specific information such as:

- Passwords
- Authentication tokens
- GitHub credentials
- Server secrets
- Databases
- Private files
- Private configuration

must not be committed to the public KMJG Hub source repository.

---

## Project Access

KMJG Hub v1 uses private project workspaces.

Only authorized members of a Project can access its internal workspace and project data.

Private project information includes:

- Project conversations
- Files
- Tasks
- Member information
- Git activity available through KMJG Hub
- Other project-specific collaboration data

Users may join a Project through:

- An invitation from an authorized member
- A valid project invite link or invite code

Public project discovery and Discord-style public communities are outside the scope of KMJG Hub v1.

A public Git repository does not automatically make its associated KMJG Hub Project public.

For example, an open-source GitHub repository may be public while the KMJG Hub development workspace associated with that repository remains private.

---

## Authentication

KMJG Hub v1 uses a hybrid authentication model.

Each self-hosted KMJG Hub Server maintains its own user accounts.

Users can authenticate using:

- A local KMJG Hub account using username/email and password
- A connected GitHub account

### Local Accounts

Users may authenticate directly with the KMJG Hub Server using their local account credentials.

Passwords must never be stored in plaintext.

The server must use an appropriate secure password hashing mechanism.

### GitHub Authentication

Users may connect their GitHub account and use GitHub as an authentication method.

GitHub integration may also be used for repository-related functionality where appropriate.

A GitHub account does not replace the user's KMJG Hub account.

The KMJG Hub Server maintains its own user identity and associates the connected GitHub identity with that account.

### Self-Hosted Identity

User accounts belong to the specific KMJG Hub Server on which they were created.

For example, an account created on:

    hub.team-a.com

is separate from an account created on:

    hub.team-b.com

KMJG Hub v1 does not require a centralized global KMJG Hub account service.

### Multi-Server Client Support

A single KMJG Hub Desktop Client may connect to multiple independent self-hosted KMJG Hub Servers.

The Client may remember Servers that the user has previously connected to.

Each Server maintains its own independent:

- User account
- Authentication session
- Projects
- Friends
- Direct Messages
- Notifications
- Profile
- Other Server-managed data

Data and authorization from one KMJG Hub Server must not grant access to another Server.

Users may switch between previously connected Servers without being required to log out of the currently active Server first.

If the Client has a valid authenticated session for the selected Server, the user may continue without logging in again.

If no valid session exists, the user must authenticate with that Server.

Removing a saved Server from the Desktop Client removes only the local saved Server connection and associated local Client state as applicable.

Removing a saved Server must not delete:

- The KMJG Hub Server
- The user's Server account
- Projects
- Server-side conversations
- Files
- Other Server-managed data

---

## Data Privacy and Retention

KMJG Hub v1 does not require end-to-end encryption.

Each self-hosted KMJG Hub instance is operated by a trusted Server Administrator who controls the server infrastructure, database, storage, and backups.

The Server Administrator may access server-side data when necessary for:

- Server administration
- Backup
- Recovery
- Maintenance
- Troubleshooting

Project roles such as Owner, Admin, or Member do not automatically grant direct access to private server-side data or other users' direct messages.

### Deleted Messages

When a user deletes a message:

- The message is hidden from normal KMJG Hub Client access.
- The server retains the deleted message temporarily using soft deletion.
- Deleted messages may be recovered by the Server Administrator when necessary.
- Deleted messages are automatically permanently removed after 30 days.
- Normal Project Members, Project Admins, and Project Owners cannot use the KMJG Hub Client to inspect deleted message contents.

Backup retention must be designed separately and documented clearly, since deleted data may remain inside existing backups until those backups expire.

### Message Deletion Permissions

KMJG Hub v1 supports deletion of user-generated messages according to the message context.

A user may delete a message that they originally sent.

For Project Chat:

- A member may delete their own Project Chat messages.
- The Project Owner or an Admin may delete user-generated Project Chat messages for moderation purposes.
- A regular Member may not delete another member's Project Chat message.

For Direct Messages:

- A user may delete only Direct Messages that they originally sent.
- Project roles such as Owner or Admin do not grant permission to delete another user's Direct Messages.
- A user may not delete a Direct Message originally sent by the other participant.

Deleting a message removes it from normal Client access for all users who would otherwise be able to view that message.

KMJG Hub v1 does not require a separate "Delete for me" message action.

Configured Git activity and Server-generated system activity are not treated as ordinary user-authored messages for message deletion permissions.

All message deletion operations must continue to follow the 30-day soft-deletion and Server Administrator recovery behavior defined above.

---

## Developer Tool Integration

KMJG Hub should integrate with development tools rather than replace them.

Potential supported tools include:

- Terminal
- VS Code
- Codex
- Claude Code
- OpenCode
- Other command-line development tools

KMJG Hub may launch supported tools using the currently selected project's local directory as context.

The terminal and IDE remain independent development tools and are not replaced by KMJG Hub.

---

## Offline Behavior

KMJG Hub v1 should provide partial offline functionality.

When disconnected from the KMJG Hub Server, users should still be able to access appropriate locally cached information such as:

- Previously loaded project information
- Previous conversations
- Downloaded files
- Last known project information
- Last known member or Git status where available

Real-time functionality requires connectivity to the KMJG Hub Server.

This includes:

- Sending messages
- Receiving new messages
- Live presence
- File transfer coordination
- Real-time notifications

When connectivity returns, the Client should synchronize with the Server where appropriate.

---

## Current v1 Non-Goals

The following features are not currently part of the KMJG Hub v1 scope:

- Voice chat
- Video calls
- Screen sharing
- Built-in AI chat or code generation
- AI-controlled automatic Git commits
- AI-controlled automatic Git pushes
- Docker management
- SSH management
- Remote desktop
- Plugin system
- Full mobile application
- Public Discord-style communities

These features may be reconsidered in future versions but should not expand the initial v1 scope.

---

## Roles and Permissions

KMJG Hub v1 defines three Project-level roles:

- Owner
- Admin
- Member

These roles apply only within a Project.

The Server Administrator is separate from Project roles and controls the self-hosted KMJG Hub Server infrastructure.

### Owner

Each Project has an Owner.

The Owner has the highest level of authority within the Project and can:

- Modify Project settings
- Invite new members
- Create invite links or invite codes
- Remove members from the Project
- Promote Members to Admin
- Demote Admins to Members
- Manage the Git repository associated with the Project
- Manage Project chat, tasks, and files where applicable
- Transfer Project ownership to another member
- Delete the Project

The Owner may leave the Project only after transferring ownership to another member.

The Owner cannot:

- Access another user's private Direct Messages
- Gain direct access to the KMJG Hub Server or database solely because they are a Project Owner

### Admin

Admins assist the Owner with Project management.

An Admin can:

- Modify general Project settings
- Invite new members
- Create invite links or invite codes
- Remove Members from the Project
- Manage regular Project members
- Manage Project chat, tasks, and files where applicable
- Manage permitted Git integration settings

An Admin cannot:

- Delete the Project
- Transfer Project ownership
- Remove or demote the Owner
- Promote or demote other Admins
- Access another user's private Direct Messages
- Gain direct access to the KMJG Hub Server or database solely because they are a Project Admin

### Member

Members are regular participants in a Project.

A Member can:

- View Project members and their presence status
- View available work context such as current branch and current task
- Participate in Project chat
- Use Direct Messages
- Send and receive files
- Accept or decline incoming file transfers
- View Git activity
- Receive Project notifications
- Create, accept, and update Tasks where permitted
- Launch configured developer tools from the Project workspace
- Leave the Project

A Member cannot:

- Modify Project settings
- Delete the Project
- Remove other members
- Change another user's Project role
- Create Project invite links or invite codes
- Change the primary Git repository associated with the Project
- Access another user's private Direct Messages
- Access the KMJG Hub Server or database through Project permissions

### Permission Principle

Project roles control what users can do inside a Project.

They do not grant infrastructure-level privileges.

Server administration remains separate from Project ownership and is controlled by the administrator of the self-hosted KMJG Hub Server.

## Project Management

### Project Creation

When a user creates a new Project, that user automatically becomes the initial Project Owner.

A Project may later be associated with its Git repository according to the supported Git integration workflow.

### Ownership Transfer

The Project Owner may transfer ownership to another Project member.

During the ownership transfer, the current Owner must select the Project role they will hold after the transfer:

- Admin
- Member

After the transfer:

- The selected member becomes the new Owner.
- The previous Owner no longer holds ownership.
- The previous Owner remains in the Project with the selected Admin or Member role.
- The Project continues to have exactly one Owner.

Ownership transfer must not remove the previous Owner from the Project automatically.

If the previous Owner wants to leave the Project, they may do so after the ownership transfer has completed.

### Leaving a Project

Members and Admins may leave a Project.

The Owner cannot leave the Project until ownership has been transferred to another member.

### Project Deletion

Only the Project Owner may delete a Project through the KMJG Hub Client.

Deleting a Project uses soft deletion.

When a Project is deleted:

- The Project becomes unavailable for normal use.
- Its KMJG Hub project data is retained for 30 days.
- The user who was the Project Owner at the time of deletion may restore the Project during the retention period.
- The Server Administrator may also restore the Project during the retention period.
- After 30 days, the Project and its retained project data are permanently deleted according to the server's retention process.

Deleting a KMJG Hub Project must not automatically delete its associated external Git repository.

For example, deleting a KMJG Hub Project associated with a GitHub repository does not delete the GitHub repository itself.

### Project Recovery

Deleted Projects should be available through a recovery area such as:

    Recently Deleted Projects

During the 30-day retention period, the user who was the Project Owner at the time of deletion may restore the Project.

Restoring the Project restores that user as the Project Owner unless a separate Server Administrator recovery procedure explicitly resolves an exceptional ownership condition.

The Server Administrator must also have a server-side recovery mechanism for deleted Projects.

Backup retention is separate from the Project soft-deletion period and will be defined independently.

---

## Project Git Integration

### Repository Setup During Project Creation

A Git repository is not required at the moment a KMJG Hub Project is created.

During Project creation, the user may choose one of the following repository setup options:

- Create a new GitHub repository
- Connect an existing GitHub repository
- Set up the Git repository later

A Project without a connected repository may still use non-Git collaboration features such as:

- Project chat
- Direct Messages
- Members
- Tasks
- File sharing

For KMJG Hub v1, one Project may be associated with a maximum of one Git repository.

### Connecting a Repository Later

If a Project does not yet have a repository, an authorized Owner or Admin may connect one later.

KMJG Hub should support:

- Selecting a repository from a connected GitHub account
- Entering an existing Git repository URL

The repository integration should not permanently depend on GitHub-specific repository URLs so that additional Git providers may be supported in the future.

### Repository Permissions

KMJG Hub Project membership and Git repository permissions are separate.

Being an Owner, Admin, or Member of a KMJG Hub Project does not automatically grant access to the associated Git repository.

Repository access remains controlled by the Git provider.

KMJG Hub should display the user's available repository access where possible and clearly indicate when the user does not have access.

### Repository Collaborator Invitations

When a Project member does not have access to the associated repository, an authorized user with sufficient Git provider permissions may invite that member to the repository through KMJG Hub.

Before sending the invitation, KMJG Hub should allow the inviter to select from the repository permission levels supported and permitted by the Git provider.

KMJG Hub must not bypass or override Git provider permissions.

### Removing Project Members

When an authorized user removes a member from a Project, KMJG Hub should provide an optional action to also remove that member's repository access.

If repository access removal fails or the acting user lacks sufficient permission, KMJG Hub must clearly report that the member was removed from the KMJG Project while their external repository access remains unchanged.

### Leaving a Project

When a Member or Admin leaves a Project, KMJG Hub may offer the user an option to also remove their own repository access where supported by the Git provider.

Failure to remove repository access must not prevent the user from leaving the KMJG Project.

### Git Push Notifications

Git push activity may be posted as system activity inside the Project Chat.

The Project Owner may configure:

- Whether Git push activity is posted to Project Chat
- Whether all Project members or selected members are notified
- Whether pushes from all branches or only selected branches trigger notifications

Git activity messages may include appropriate information such as:

- User who pushed
- Branch
- Number of commits
- Commit summaries

Repository permissions and privacy must still be respected when displaying Git information.

---

## Project Invitations

### Inviting Members

Project Owners and Admins may invite users to a Project using:

- Direct invitation to an existing user on the same KMJG Hub Server
- Project invite link
- Project invite code

New users joining a Project receive the Member role by default.

### Direct Invitations

An Owner or Admin may directly invite an existing user on the same KMJG Hub Server.

The recipient must explicitly:

- Accept the invitation
- Decline the invitation

The inviter may configure the invitation expiration period, such as:

- 1 hour
- 1 day
- 7 days
- 30 days
- No expiration

The inviter may cancel a pending invitation before it is accepted.

### Invite Links and Codes

An Owner or Admin may create a Project invite link or invite code.

Invite links and codes may be configured with:

- An expiration period
- A maximum number of uses
- Unlimited uses where permitted

An Owner or Admin may revoke an active invite link or code at any time.

A user who intentionally uses a valid invite link or code may join the Project immediately without additional Owner or Admin approval.

If the user does not yet have an account on that KMJG Hub Server, they must register or log in before joining.

KMJG Hub v1 does not require built-in email invitation delivery. Invite links and codes may be shared using external communication tools.

---

## Friends and Direct Messages

### Server-Level Relationships

Friends and Direct Messages exist at the KMJG Hub Server account level rather than belonging to a specific Project.

Leaving a Project does not automatically remove friendships.

Users who remain friends may continue using Direct Messages even when they no longer share a Project.

### Friend Requests

Users on the same KMJG Hub Server may send Friend Requests.

A Friend Request must be explicitly accepted or declined by the recipient.

Users do not need to become friends before joining the same Project.

### Project Member Messaging

Users who share a Project may send Direct Messages to each other without first becoming friends.

If two users no longer share a Project, they may continue Direct Messaging only if another permitted relationship exists, such as an accepted friendship.

A user who does not share a Project with another user and is not their friend cannot directly message that user.

They may send a Friend Request instead.

### Friends and Project Invitations

The Project invitation interface should make it easy to invite existing friends.

An Owner or Admin may:

- Select a user from their Friends list
- Search for an existing user on the same KMJG Hub Server
- Create an invite link or invite code

Friendship is not required for Project membership and does not grant access to any Project automatically.

### Blocking

Users may block other users.

When a user is blocked:

- Direct Messages between the two users are disabled
- Friend Requests between the two users are disabled
- Any existing friendship between the two users is removed
- The blocked user may be informed that they have been blocked

Blocking affects personal communication but does not alter Project permissions.

If both users remain members of the same Project, Project collaboration information remains available according to their Project permissions, including:

- Project Chat
- Tasks
- Git activity
- Project files

Unblocking a user does not automatically restore the previous friendship.

A new Friend Request is required to become friends again.

---

## Presence and Work Status

### Presence Status

KMJG Hub maintains automatic presence information for users.

The basic presence states in v1 are:

- Online
- Offline

Presence and work status are separate concepts.

### Work Status

A user may be Online while also having a Working status.

Working status may be activated:

- Automatically when KMJG Hub detects supported Project-related development activity
- Manually by the user using actions such as Start Working or Stop Working

Automatic detection must not prevent the user from manually overriding their Working status.

When appropriate, a working member may expose Project-related context such as:

- Current Branch
- Current Task

When a user becomes Offline, KMJG Hub must not continue presenting Working, Current Branch, or Current Task as current live status.

---

## User Profiles and Privacy

### Basic Profile

A KMJG Hub user profile may contain:

- Display Name
- Username
- Avatar
- Bio
- Presence Status
- Work Status
- Current Project
- Current Task
- Current Branch
- Connected Git provider account
- Repositories the user chooses to expose

### Field-Level Privacy

Users control the visibility of individual profile fields.

Different profile fields may use different visibility settings.

Available visibility audiences may include:

- Everyone on the same KMJG Hub Server
- Friends
- Project Members where relevant
- Friends and Project Members
- Nobody

KMJG Hub should expose only the profile information permitted by the user's privacy settings.

Users who do not have permission to view the full profile may still see the minimum information that the profile owner has chosen to make visible, such as an Avatar or Bio.

### Repository Profile Visibility

Users may choose whether connected repositories are displayed on their profile.

If a user attempts to expose a repository that the Git provider identifies as Private, KMJG Hub must clearly warn the user before enabling that visibility.

The warning should explain that displaying the repository may reveal information such as its name or other explicitly selected metadata to the permitted profile audience.

Displaying a Private repository on a KMJG Hub profile:

- Does not make the external repository Public
- Does not grant repository access to profile viewers
- Does not automatically expose repository files, branches, commits, or other private repository contents

Only information explicitly permitted by the user and allowed by the applicable privacy rules should be displayed.

---

## Task Management

### Task Model

Project members may create Tasks.

A Task in KMJG Hub v1 contains:

- Title
- Description
- Status
- Assignee
- Creator
- Optional Due Date

### Task Status

KMJG Hub v1 supports:

- To Do
- In Progress
- Done

If a To Do Task is selected as a user's Current Task, KMJG Hub may automatically move that Task to In Progress.

Completing a Task requires the user to mark it as Done.

### Task Assignment

Project members may assign Tasks to themselves.

Self-assigned Tasks do not require additional acceptance.

When one member assigns a Task to another member, the recipient must be able to:

- Accept the assignment
- Decline the assignment

A user may have multiple assigned Tasks but may have only one Current Task at a time.

### Current Task

A user may select one of their assigned Tasks as their Current Task.

The Current Task may be shown as part of the user's work context according to Project permissions and the user's profile privacy settings.

### Task Comments

Project members may read and post comments on Project Tasks.

Task Comments provide a place for discussion directly related to a specific Task.

KMJG Hub v1 does not require file attachments inside Task Comments.

---

## Notifications

KMJG Hub v1 should provide notifications for important collaboration events without notifying users about every routine activity.

Relevant notification events may include:

- A Task is assigned to the user
- A new comment is posted on a relevant Task
- A Direct Message is received
- A file transfer request is received
- A Project invitation is received
- A user's Project role changes
- Configured Git push activity occurs

Project Chat messages do not require individual notifications for every message by default.

Git push notifications follow the Project Git notification configuration defined in the Git integration requirements.

---

## File Sharing and Transfer

### Direct File Transfer

Users may directly send files to another permitted user.

Before the file content is transferred, the recipient must receive a File Transfer Request containing appropriate information such as:

- Sender
- File name
- File size

The recipient must explicitly:

- Accept the transfer
- Decline the transfer

The file must not be uploaded to server storage before the recipient accepts the Direct File Transfer.

Declining a Direct File Transfer must not cause the file to be downloaded or retained on the KMJG Hub Server.

After acceptance, KMJG Hub may begin the transfer according to the supported transfer architecture.

The transfer interface should display progress and allow cancellation while a transfer is active.

If a transfer is interrupted, KMJG Hub v1 may report the transfer as interrupted without requiring resumable file transfer support.

The exact transport mechanism for Direct File Transfer will be defined separately in the architecture specification.

### Project Chat Attachments

Project members may send files as attachments in Project Chat.

Unlike Direct File Transfer, Project Chat attachments are stored by the KMJG Hub Server so authorized Project members may download them later.

Downloading a Project Chat attachment to a user's local device occurs only when requested by that user.

### Chat Attachment Retention

A Project Chat attachment is associated with the message containing that attachment.

When the associated message is deleted:

- The attachment becomes unavailable through normal Client access
- The attachment enters the applicable 30-day soft-deletion retention period
- The attachment is permanently removed from normal server storage after the retention period

Backup copies may remain until the applicable backup expires.

### File and Storage Limits

KMJG Hub does not require one universal hardcoded file-size or Project-storage limit for every self-hosted deployment.

The Server Administrator may configure limits such as:

- Maximum size of an individual uploaded file
- Maximum storage available to a Project

The KMJG Hub Client should communicate applicable server limits to users before or during file-sharing operations where appropriate.

Direct transfers that do not consume persistent server storage should not count against Project storage limits unless required by the final transfer architecture.

---

## Backup Retention

Backup retention is independent from normal soft-deletion retention.

The Server Administrator controls the backup retention policy for their self-hosted KMJG Hub deployment.

Server backup configuration may include:

- Whether automatic backups are enabled
- Backup frequency
- Backup retention duration
- Maximum number of retained backups
- Backup storage location
- Deployment-specific backup limits or policies

A Server Administrator may configure an appropriate retention period according to the deployment's storage capacity and operational requirements.

KMJG Hub documentation and administration interfaces must clearly communicate that data deleted from normal application storage may continue to exist inside previously created backups until those backups expire or are removed according to the server's backup policy.
