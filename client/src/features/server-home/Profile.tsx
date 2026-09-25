import { FormEvent, useEffect, useState } from "react";
import {
  ApiError,
  PrivacyAudience,
  ProfileField,
  UserProfile,
  getOwnProfile,
  isSessionExpired,
  setProfilePrivacy,
  updateProfile,
} from "../../lib/apiClient";
import GitHubConnection from "./GitHubConnection";
import "./Profile.css";

interface ProfileProps {
  serverUrl: string;
  token: string;
  onBack: () => void;
  onSessionExpired: () => void;
}

const PROFILE_FIELDS: ProfileField[] = [
  "display_name",
  "avatar",
  "bio",
  "current_project",
  "current_task",
  "current_branch",
  "repositories",
];

const FIELD_LABELS: Record<ProfileField, string> = {
  display_name: "Display Name",
  avatar: "Avatar",
  bio: "Bio",
  current_project: "Current Project",
  current_task: "Current Task",
  current_branch: "Current Branch",
  repositories: "Repositories",
};

const AUDIENCE_OPTIONS: { value: PrivacyAudience; label: string }[] = [
  { value: "everyone", label: "Everyone" },
  { value: "friends", label: "Friends" },
  { value: "project_members", label: "Project Members" },
  { value: "friends_and_project_members", label: "Friends & Project Members" },
  { value: "nobody", label: "Only Me" },
];

const DEFAULT_AUDIENCE: Record<ProfileField, PrivacyAudience> = {
  display_name: "everyone",
  avatar: "everyone",
  bio: "friends",
  current_project: "everyone",
  current_task: "friends",
  current_branch: "friends",
  repositories: "everyone",
};

function privacyMapFrom(profile: UserProfile): Record<ProfileField, PrivacyAudience> {
  const map = { ...DEFAULT_AUDIENCE };
  for (const setting of profile.privacy ?? []) {
    map[setting.field] = setting.audience;
  }
  return map;
}

function Profile({ serverUrl, token, onBack, onSessionExpired }: ProfileProps) {
  const [profile, setProfile] = useState<UserProfile | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);

  const [displayName, setDisplayName] = useState("");
  const [avatar, setAvatar] = useState("");
  const [bio, setBio] = useState("");
  const [savingProfile, setSavingProfile] = useState(false);
  const [profileError, setProfileError] = useState<string | null>(null);
  const [profileSaved, setProfileSaved] = useState(false);

  const [privacy, setPrivacy] = useState<Record<ProfileField, PrivacyAudience>>(DEFAULT_AUDIENCE);
  const [savingPrivacy, setSavingPrivacy] = useState(false);
  const [privacyError, setPrivacyError] = useState<string | null>(null);
  const [privacySaved, setPrivacySaved] = useState(false);

  useEffect(() => {
    let cancelled = false;

    getOwnProfile(serverUrl, token)
      .then((loaded) => {
        if (cancelled) return;
        setProfile(loaded);
        setDisplayName(loaded.display_name ?? "");
        setAvatar(loaded.avatar ?? "");
        setBio(loaded.bio ?? "");
        setPrivacy(privacyMapFrom(loaded));
      })
      .catch((err) => {
        if (cancelled) return;
        if (isSessionExpired(err)) {
          onSessionExpired();
          return;
        }
        setLoadError(err instanceof ApiError ? err.message : "Could not load your profile.");
      });

    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverUrl, token]);

  async function handleSaveProfile(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setProfileError(null);
    setProfileSaved(false);
    setSavingProfile(true);
    try {
      const updated = await updateProfile(serverUrl, token, {
        display_name: displayName,
        avatar,
        bio,
      });
      setProfile(updated);
      setPrivacy(privacyMapFrom(updated));
      setProfileSaved(true);
    } catch (err) {
      if (isSessionExpired(err)) {
        onSessionExpired();
        return;
      }
      setProfileError(err instanceof ApiError ? err.message : "Could not save your profile.");
    } finally {
      setSavingProfile(false);
    }
  }

  async function handleSavePrivacy(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setPrivacyError(null);
    setPrivacySaved(false);
    setSavingPrivacy(true);
    try {
      const settings = PROFILE_FIELDS.map((field) => ({ field, audience: privacy[field] }));
      const updated = await setProfilePrivacy(serverUrl, token, settings);
      setProfile(updated);
      setPrivacy(privacyMapFrom(updated));
      setPrivacySaved(true);
    } catch (err) {
      if (isSessionExpired(err)) {
        onSessionExpired();
        return;
      }
      setPrivacyError(err instanceof ApiError ? err.message : "Could not save your privacy settings.");
    } finally {
      setSavingPrivacy(false);
    }
  }

  return (
    <main className="profile">
      <div className="profile__header">
        <h1>Your Profile</h1>
        <button type="button" onClick={onBack}>
          Back
        </button>
      </div>

      {loadError && (
        <p className="profile__error" role="alert">
          {loadError}
        </p>
      )}

      {profile === null && !loadError && <p className="profile__loading">Loading your profile...</p>}

      {profile !== null && (
        <>
          <section className="profile__section">
            <h2>Details</h2>
            <p className="profile__meta">
              Signed in as <strong>{profile.username}</strong> ({profile.email})
            </p>

            <form className="profile__form" onSubmit={handleSaveProfile} noValidate>
              <label className="profile__label" htmlFor="profile-display-name">
                Display Name
              </label>
              <input
                id="profile-display-name"
                type="text"
                value={displayName}
                onChange={(event) => setDisplayName(event.target.value)}
                placeholder={profile.username}
              />

              <label className="profile__label" htmlFor="profile-avatar">
                Avatar URL
              </label>
              <input
                id="profile-avatar"
                type="text"
                value={avatar}
                onChange={(event) => setAvatar(event.target.value)}
                placeholder="https://example.com/avatar.png"
              />

              <label className="profile__label" htmlFor="profile-bio">
                Bio
              </label>
              <textarea
                id="profile-bio"
                value={bio}
                onChange={(event) => setBio(event.target.value)}
                rows={3}
                maxLength={500}
              />

              {profileError && (
                <p className="profile__error" role="alert">
                  {profileError}
                </p>
              )}
              {profileSaved && !profileError && <p className="profile__saved">Saved.</p>}

              <div className="profile__buttons">
                <button type="submit" disabled={savingProfile}>
                  {savingProfile ? "Saving..." : "Save Profile"}
                </button>
              </div>
            </form>
          </section>

          <section className="profile__section">
            <h2>Privacy</h2>
            <p className="profile__meta">Choose who can see each field of your profile.</p>

            <form className="profile__form" onSubmit={handleSavePrivacy} noValidate>
              <div className="profile__privacy-grid">
                {PROFILE_FIELDS.map((field) => (
                  <div key={field} className="profile__privacy-row">
                    <label className="profile__label" htmlFor={`privacy-${field}`}>
                      {FIELD_LABELS[field]}
                    </label>
                    <select
                      id={`privacy-${field}`}
                      value={privacy[field]}
                      onChange={(event) =>
                        setPrivacy((current) => ({ ...current, [field]: event.target.value as PrivacyAudience }))
                      }
                    >
                      {AUDIENCE_OPTIONS.map((option) => (
                        <option key={option.value} value={option.value}>
                          {option.label}
                        </option>
                      ))}
                    </select>
                  </div>
                ))}
              </div>

              {privacyError && (
                <p className="profile__error" role="alert">
                  {privacyError}
                </p>
              )}
              {privacySaved && !privacyError && <p className="profile__saved">Saved.</p>}

              <div className="profile__buttons">
                <button type="submit" disabled={savingPrivacy}>
                  {savingPrivacy ? "Saving..." : "Save Privacy Settings"}
                </button>
              </div>
            </form>
          </section>

          <GitHubConnection serverUrl={serverUrl} token={token} onSessionExpired={onSessionExpired} />
        </>
      )}
    </main>
  );
}

export default Profile;
