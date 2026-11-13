"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { ApiError, clientApi } from "@/lib/api";
import { formatBytes } from "@/lib/format";
import type { Self } from "@/lib/types";

import Avatar from "./Avatar";

/** SettingsForm holds the three account forms and the session controls. */
export default function SettingsForm({
  user,
  maxUploadBytes,
}: {
  user: Self;
  maxUploadBytes: number;
}) {
  const router = useRouter();

  const [displayName, setDisplayName] = useState(user.display_name);
  const [bio, setBio] = useState(user.bio);
  const [avatarUrl, setAvatarUrl] = useState(user.avatar_url);
  const [savingProfile, setSavingProfile] = useState(false);
  const [profileMessage, setProfileMessage] = useState<{ ok: boolean; text: string } | null>(null);

  const [email, setEmail] = useState(user.email);
  const [savingEmail, setSavingEmail] = useState(false);
  const [emailMessage, setEmailMessage] = useState<{ ok: boolean; text: string } | null>(null);

  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [savingPassword, setSavingPassword] = useState(false);
  const [passwordMessage, setPasswordMessage] = useState<{ ok: boolean; text: string } | null>(null);

  async function saveProfile() {
    setSavingProfile(true);
    setProfileMessage(null);
    try {
      await clientApi<Self>("/api/v1/me", {
        method: "PATCH",
        body: { display_name: displayName, bio, avatar_url: avatarUrl },
      });
      setProfileMessage({ ok: true, text: "Profile saved." });
      router.refresh();
    } catch (err) {
      setProfileMessage({
        ok: false,
        text: err instanceof ApiError ? err.message : "could not save the profile",
      });
    } finally {
      setSavingProfile(false);
    }
  }

  async function uploadAvatar(file: File) {
    if (maxUploadBytes > 0 && file.size > maxUploadBytes) {
      setProfileMessage({
        ok: false,
        text: `That image is larger than ${formatBytes(maxUploadBytes)}.`,
      });
      return;
    }
    setSavingProfile(true);
    setProfileMessage(null);
    try {
      const body = new FormData();
      body.append("file", file);
      const response = await fetch("/api/v1/uploads", { method: "POST", body });
      const payload = (await response.json()) as
        | { upload: { url: string } }
        | { error?: { message?: string } };
      if (!response.ok || !("upload" in payload)) {
        throw new Error(
          "error" in payload && payload.error?.message
            ? payload.error.message
            : "could not upload the image",
        );
      }
      setAvatarUrl(payload.upload.url);
    } catch (err) {
      setProfileMessage({
        ok: false,
        text: err instanceof Error ? err.message : "could not upload the image",
      });
    } finally {
      setSavingProfile(false);
    }
  }

  async function saveEmail() {
    setSavingEmail(true);
    setEmailMessage(null);
    try {
      await clientApi<Self>("/api/v1/me/email", { method: "POST", body: { email } });
      setEmailMessage({
        ok: true,
        text: email ? "Saved. Check your inbox for a confirmation link." : "E-mail removed.",
      });
      router.refresh();
    } catch (err) {
      setEmailMessage({
        ok: false,
        text: err instanceof ApiError ? err.message : "could not save the address",
      });
    } finally {
      setSavingEmail(false);
    }
  }

  async function savePassword() {
    setSavingPassword(true);
    setPasswordMessage(null);
    try {
      await clientApi("/api/v1/me/password", {
        method: "POST",
        body: { current_password: currentPassword, new_password: newPassword },
      });
      setCurrentPassword("");
      setNewPassword("");
      setPasswordMessage({ ok: true, text: "Password changed. Other devices were signed out." });
    } catch (err) {
      setPasswordMessage({
        ok: false,
        text: err instanceof ApiError ? err.message : "could not change the password",
      });
    } finally {
      setSavingPassword(false);
    }
  }

  async function signOutOthers() {
    try {
      await clientApi("/api/v1/me/sign-out-others", { method: "POST" });
      setPasswordMessage({ ok: true, text: "Every other device was signed out." });
    } catch {
      setPasswordMessage({ ok: false, text: "could not sign out other sessions" });
    }
  }

  return (
    <div className="stack" style={{ maxWidth: "40rem" }}>
      {/* ---- profile ---- */}
      <section className="card card-pad stack-sm">
        <h2 style={{ fontSize: "1.05rem" }}>Profile</h2>
        {profileMessage && (
          <div className={profileMessage.ok ? "form-success" : "form-error"}>
            {profileMessage.text}
          </div>
        )}

        <div className="row" style={{ gap: "1rem" }}>
          <Avatar
            user={{ ...user, avatar_url: avatarUrl, display_name: displayName }}
            size="xl"
          />
          <div className="stack-sm" style={{ flex: 1 }}>
            <div className="field">
              <label htmlFor="avatar-url">Avatar URL</label>
              <input
                id="avatar-url"
                className="input"
                value={avatarUrl}
                onChange={(event) => setAvatarUrl(event.target.value)}
                placeholder="https://…"
              />
              <span className="hint">
                Uploading an image is preferred. A remote URL only works if this instance allows
                them, because every reader&apos;s browser would otherwise call that host.
              </span>
            </div>
            <label className="btn btn-sm" style={{ cursor: "pointer", alignSelf: "flex-start" }}>
              Upload image
              <input
                type="file"
                accept="image/png,image/jpeg,image/gif,image/webp"
                hidden
                onChange={(event) => {
                  const file = event.target.files?.[0];
                  if (file) void uploadAvatar(file);
                  event.target.value = "";
                }}
              />
            </label>
          </div>
        </div>

        <div className="field">
          <label htmlFor="display-name">Display name</label>
          <input
            id="display-name"
            className="input"
            value={displayName}
            onChange={(event) => setDisplayName(event.target.value)}
            maxLength={60}
            placeholder={user.username}
          />
          <span className="hint">Shown instead of your username. Leave empty to use @{user.username}.</span>
        </div>

        <div className="field">
          <label htmlFor="bio">Bio</label>
          <textarea
            id="bio"
            className="textarea"
            style={{ minHeight: "5rem" }}
            value={bio}
            onChange={(event) => setBio(event.target.value)}
            maxLength={600}
          />
        </div>

        <div className="row">
          <button type="button" className="btn btn-primary" onClick={saveProfile} disabled={savingProfile}>
            {savingProfile ? "Saving…" : "Save profile"}
          </button>
        </div>
      </section>

      {/* ---- email ---- */}
      <section className="card card-pad stack-sm">
        <h2 style={{ fontSize: "1.05rem" }}>E-mail</h2>
        {emailMessage && (
          <div className={emailMessage.ok ? "form-success" : "form-error"}>{emailMessage.text}</div>
        )}
        <div className="field">
          <label htmlFor="email">Address</label>
          <input
            id="email"
            className="input"
            type="email"
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            autoComplete="email"
          />
          <span className="hint">
            {user.email_verified
              ? "Confirmed."
              : "Not confirmed — used for sign-in links and password recovery."}
          </span>
        </div>
        <div className="row">
          <button type="button" className="btn" onClick={saveEmail} disabled={savingEmail}>
            {savingEmail ? "Saving…" : "Save address"}
          </button>
        </div>
      </section>

      {/* ---- password ---- */}
      <section className="card card-pad stack-sm">
        <h2 style={{ fontSize: "1.05rem" }}>Password</h2>
        {passwordMessage && (
          <div className={passwordMessage.ok ? "form-success" : "form-error"}>
            {passwordMessage.text}
          </div>
        )}

        {user.has_password && (
          <div className="field">
            <label htmlFor="current-password">Current password</label>
            <input
              id="current-password"
              className="input"
              type="password"
              value={currentPassword}
              onChange={(event) => setCurrentPassword(event.target.value)}
              autoComplete="current-password"
            />
          </div>
        )}

        <div className="field">
          <label htmlFor="new-password">
            {user.has_password ? "New password" : "Set a password"}
          </label>
          <input
            id="new-password"
            className="input"
            type="password"
            value={newPassword}
            onChange={(event) => setNewPassword(event.target.value)}
            autoComplete="new-password"
            minLength={10}
          />
          <span className="hint">
            {user.has_password
              ? "Changing the password signs out your other devices."
              : "You signed up with a provider. Set a password to be able to sign in without it."}
          </span>
        </div>

        <div className="row">
          <button
            type="button"
            className="btn btn-primary"
            onClick={savePassword}
            disabled={savingPassword || newPassword.length < 10}
          >
            {savingPassword ? "Saving…" : user.has_password ? "Change password" : "Set password"}
          </button>
          <button type="button" className="btn btn-ghost" onClick={signOutOthers}>
            Sign out other devices
          </button>
        </div>
      </section>

      {/* ---- linked accounts ---- */}
      {user.providers.length > 0 && (
        <section className="card card-pad stack-sm">
          <h2 style={{ fontSize: "1.05rem" }}>Linked accounts</h2>
          <div className="row" style={{ gap: "0.4rem" }}>
            {user.providers.map((provider) => (
              <span className="badge badge-ok" key={provider}>
                {provider}
              </span>
            ))}
          </div>
        </section>
      )}
    </div>
  );
}
