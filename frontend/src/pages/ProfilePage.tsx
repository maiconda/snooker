import { ChangeEvent, FormEvent, useEffect, useMemo, useState } from "react";
import { useAuth } from "../auth/AuthProvider";
import { AuthApiError } from "../auth/types";
import { Button } from "../components/Button";
import { Notice } from "../components/Notice";
import { TextField } from "../components/TextField";
import { navigate } from "../lib/router";
import * as profileApi from "../profile/profileApi";
import type { Profile } from "../profile/types";

const OUTPUT_SIZE = 512;
const OUTPUT_TYPE = "image/webp";
const OUTPUT_QUALITY = 0.86;

export function ProfilePage() {
  const auth = useAuth();
  const session = auth.session;
  const isOnboarding = session?.status === "onboarding_pending";
  const [profile, setProfile] = useState<Profile | null>(null);
  const [nickname, setNickname] = useState("");
  const [bio, setBio] = useState("");
  const [imageUrl, setImageUrl] = useState<string | null>(null);
  const [imageName, setImageName] = useState("");
  const [zoom, setZoom] = useState(1);
  const [offsetX, setOffsetX] = useState(0);
  const [offsetY, setOffsetY] = useState(0);
  const [loading, setLoading] = useState(session?.status === "active");
  const [saving, setSaving] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!session?.accessToken || session.status !== "active") {
      return;
    }

    let active = true;
    setLoading(true);
    profileApi
      .getMyProfile(session.accessToken)
      .then((loadedProfile) => {
        if (!active) {
          return;
        }
        setProfile(loadedProfile);
        setNickname(loadedProfile.nickname);
        setBio(loadedProfile.bio);
      })
      .catch((caught: unknown) => {
        if (!active) {
          return;
        }
        if (caught instanceof AuthApiError && caught.status === 404) {
          setProfile(null);
          return;
        }
        setError(caught instanceof Error ? caught.message : "Nao foi possivel carregar o perfil.");
      })
      .finally(() => {
        if (active) {
          setLoading(false);
        }
      });

    return () => {
      active = false;
    };
  }, [session?.accessToken, session?.status]);

  useEffect(() => {
    return () => {
      if (imageUrl) {
        URL.revokeObjectURL(imageUrl);
      }
    };
  }, [imageUrl]);

  const nicknameIssue = useMemo(() => validateNickname(nickname), [nickname]);
  const bioIssue = useMemo(() => (bio.length > 200 ? "A bio deve ter no maximo 200 caracteres." : null), [bio]);
  const needsNewPhoto = isOnboarding || (!profile?.photo_url && !imageUrl);
  const canSave = Boolean(session?.accessToken && nickname && !nicknameIssue && !bioIssue && !saving && (!needsNewPhoto || imageUrl));

  function handleFile(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    if (!file) {
      return;
    }
    if (!["image/jpeg", "image/png", "image/webp"].includes(file.type)) {
      setError("Use uma imagem JPG, PNG ou WebP.");
      return;
    }
    if (imageUrl) {
      URL.revokeObjectURL(imageUrl);
    }
    setImageName(file.name);
    setImageUrl(URL.createObjectURL(file));
    setZoom(1);
    setOffsetX(0);
    setOffsetY(0);
    setError(null);
    setNotice(null);
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!session?.accessToken || !canSave) {
      return;
    }

    setSaving(true);
    setError(null);
    setNotice(null);

    try {
      let photoUploadId: string | undefined;
      if (imageUrl) {
        const blob = await cropImageToSquareBlob(imageUrl, zoom, offsetX, offsetY);
        const upload = await profileApi.createPhotoUploadURL(session.accessToken, blob.type, blob.size);
        await profileApi.uploadPhoto(upload.upload_url, blob);
        photoUploadId = upload.upload_id;
      }

      if (isOnboarding) {
        if (!photoUploadId) {
          setError("Escolha uma foto para concluir o perfil.");
          return;
        }
        const response = await profileApi.completeProfile(session.accessToken, {
          nickname,
          bio,
          photo_upload_id: photoUploadId
        });
        auth.acceptAccessToken(response.access_token);
        setProfile(response.profile);
        navigate("/");
        return;
      }

      const updated = await profileApi.updateProfile(session.accessToken, {
        nickname,
        bio,
        photo_upload_id: photoUploadId
      });
      setProfile(updated);
      setImageUrl(null);
      setImageName("");
      setNotice("Perfil atualizado.");
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Nao foi possivel salvar o perfil.");
    } finally {
      setSaving(false);
    }
  }

  const previewImage = imageUrl ?? profile?.photo_url ?? "";
  const totalXP = profile?.xp ?? 0;
  const level = Math.floor(totalXP / 100) + 1;
  const nextLevelXP = level * 100;
  const levelProgress = totalXP % 100;

  return (
    <main className="relative min-h-screen bg-[radial-gradient(ellipse_at_top,_var(--tw-gradient-stops))] from-red-100/30 via-zinc-50 to-zinc-50 dark:from-red-950/20 dark:via-neutral-950 dark:to-neutral-950 p-6 text-neutral-900 dark:text-white transition-colors duration-300 animate-fade-in">
      <div className="absolute inset-0 bg-[linear-gradient(to_bottom,rgba(0,0,0,0.015)_1px,transparent_1px)] dark:bg-[linear-gradient(to_bottom,rgba(255,255,255,0.005)_1px,transparent_1px)] bg-[size:100%_40px] pointer-events-none" />
      <section className="relative mx-auto flex min-h-[calc(100vh-3rem)] w-full max-w-5xl flex-col justify-center px-5">
        <header className="mb-6 flex items-center justify-between border-b border-neutral-200 dark:border-white/10 pb-4">
          <div>
            <h1 className="mt-1 text-2xl font-bold tracking-tight text-neutral-900 dark:text-white">{isOnboarding ? "Criar perfil" : "Editar perfil"}</h1>
          </div>
          {!isOnboarding ? (
            <Button onClick={() => navigate("/")} variant="outline" className="!w-auto px-4 h-9">
              Voltar
            </Button>
          ) : null}
        </header>

        <div className="grid flex-1 gap-6 md:grid-cols-[320px_1fr]">
          <aside className="border border-neutral-200 dark:border-white/10 bg-white/40 dark:bg-zinc-900/30 p-4 rounded-xl shadow-xl shadow-neutral-200/10 dark:shadow-none">
            <div className="aspect-square w-full overflow-hidden border border-neutral-200 dark:border-white/10 bg-neutral-100 dark:bg-neutral-800 rounded-lg">
              {previewImage ? (
                imageUrl ? (
                  <img
                    alt=""
                    className="h-full w-full object-cover"
                    src={previewImage}
                    style={{
                      transform: `translate(${offsetX}%, ${offsetY}%) scale(${zoom})`,
                      transformOrigin: "center"
                    }}
                  />
                ) : (
                  <img alt="" className="h-full w-full object-cover" src={previewImage} />
                )
              ) : (
                <div className="flex h-full w-full items-center justify-center bg-neutral-100 dark:bg-neutral-800/40 text-neutral-400 dark:text-neutral-500">
                  <svg className="h-16 w-16 opacity-60" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M15.75 6a3.75 3.75 0 1 1-7.5 0 3.75 3.75 0 0 1 7.5 0ZM4.501 20.118a7.5 7.5 0 0 1 14.998 0A17.933 17.933 0 0 1 12 21.75c-2.676 0-5.216-.584-7.499-1.632Z" />
                  </svg>
                </div>
              )}
            </div>

            <div className="mt-4 border-t border-neutral-200 dark:border-white/10 pt-4">
              <p className="text-sm font-bold text-neutral-800 dark:text-white break-words">{nickname || "nickname"}</p>
              <p className="mt-1 min-h-10 text-sm text-neutral-500 dark:text-neutral-400 break-words line-clamp-3">{bio || "bio"}</p>
              <div className="mt-4 border border-neutral-200 dark:border-white/10 bg-white/80 dark:bg-white/5 p-3 rounded-lg shadow-sm">
                <div className="flex items-center justify-between text-sm">
                  <span className="font-semibold text-neutral-800 dark:text-white">Nível {level}</span>
                  <span className="text-neutral-500 dark:text-neutral-400">{totalXP} XP</span>
                </div>
                <div className="mt-3 h-2 bg-neutral-200 dark:bg-neutral-800 rounded-full overflow-hidden">
                  <div className="h-full bg-gradient-to-r from-red-600 to-rose-500 rounded-full shadow-[0_0_8px_rgba(239,68,68,0.5)]" style={{ width: `${levelProgress}%` }} />
                </div>
                <p className="mt-2 text-sm text-neutral-500 dark:text-neutral-400 font-semibold">{nextLevelXP - totalXP} XP para o próximo nível</p>
              </div>
            </div>
          </aside>

          <form className="border border-neutral-200 dark:border-white/10 bg-white/80 dark:bg-zinc-900/30 p-6 rounded-xl text-neutral-900 dark:text-white shadow-xl shadow-neutral-200/10 dark:shadow-none" onSubmit={handleSubmit}>
            {loading ? (
              <div className="h-80 animate-pulse bg-neutral-100" />
            ) : (
              <div className="space-y-4">
                <TextField
                  label="Nickname"
                  name="nickname"
                  autoComplete="nickname"
                  value={nickname}
                  onChange={(event) => setNickname(event.target.value)}
                  maxLength={24}
                  required
                />
                {nicknameIssue ? <p className="-mt-3 text-sm text-red-600">{nicknameIssue}</p> : null}

                <label className="block text-sm text-neutral-700 dark:text-neutral-300" htmlFor="bio">
                  <span className="mb-2 block font-medium text-neutral-600 dark:text-neutral-400">Bio</span>
                  <textarea
                    id="bio"
                    name="bio"
                    className="min-h-20 w-full resize-none rounded-lg border border-neutral-300 dark:border-white/10 bg-white dark:bg-zinc-900/50 px-3 py-2 text-sm text-neutral-900 dark:text-white placeholder-neutral-400 dark:placeholder-neutral-600 outline-none transition focus:border-red-500/60 focus:ring-2 focus:ring-red-500/20"
                    maxLength={200}
                    value={bio}
                    onChange={(event) => setBio(event.target.value)}
                  />
                </label>
                <div className="-mt-3 flex justify-between text-sm font-semibold text-neutral-500 dark:text-neutral-400">
                  <span>{bioIssue ?? ""}</span>
                  <span>{bio.length}/200</span>
                </div>

                <label className="block text-sm text-neutral-700 dark:text-neutral-300" htmlFor="photo">
                  <span className="mb-2 block font-medium text-neutral-600 dark:text-neutral-400">Foto</span>
                  <input
                    id="photo"
                    name="photo"
                    type="file"
                    accept="image/jpeg,image/png,image/webp"
                    className="block w-full border border-neutral-300 dark:border-white/10 rounded-lg bg-white dark:bg-zinc-900/50 text-sm text-neutral-700 dark:text-neutral-300 file:mr-4 file:h-10 file:border-0 file:bg-red-600 file:px-4 file:text-sm file:font-medium file:text-white file:cursor-pointer hover:file:bg-red-500 file:transition"
                    onChange={handleFile}
                  />
                </label>
                {imageName ? <p className="-mt-3 text-sm text-neutral-500 font-medium">{imageName}</p> : null}

                {imageUrl ? (
                  <div className="grid gap-4 border border-neutral-200 dark:border-white/5 bg-neutral-50 dark:bg-neutral-950/40 p-4 rounded-lg md:grid-cols-3">
                    <RangeField label="Zoom" min={1} max={2.5} step={0.05} value={zoom} onChange={setZoom} />
                    <RangeField label="Horizontal" min={-40} max={40} step={1} value={offsetX} onChange={setOffsetX} />
                    <RangeField label="Vertical" min={-40} max={40} step={1} value={offsetY} onChange={setOffsetY} />
                  </div>
                ) : null}

                <Notice message={error} />
                <Notice message={notice} />

                <div className="flex flex-col gap-3 sm:flex-row">
                  <Button type="submit" disabled={!canSave}>
                    {saving ? "Salvando..." : isOnboarding ? "Concluir perfil" : "Salvar perfil"}
                  </Button>
                  {!isOnboarding ? (
                    <Button type="button" variant="outline" onClick={() => navigate("/")}>
                      Cancelar
                    </Button>
                  ) : null}
                </div>
              </div>
            )}
          </form>
        </div>
      </section>
    </main>
  );
}

function RangeField({
  label,
  value,
  min,
  max,
  step,
  onChange
}: {
  label: string;
  value: number;
  min: number;
  max: number;
  step: number;
  onChange: (value: number) => void;
}) {
  return (
    <label className="block text-sm font-bold uppercase tracking-[0.14em] text-neutral-500 dark:text-neutral-400">
      <span className="mb-2 block">{label}</span>
      <input
        className="w-full accent-red-600 dark:accent-red-500 cursor-pointer"
        max={max}
        min={min}
        step={step}
        type="range"
        value={value}
        onChange={(event) => onChange(Number(event.target.value))}
      />
    </label>
  );
}

function validateNickname(value: string): string | null {
  if (!value) {
    return null;
  }
  if (!/^[A-Za-z0-9_]{3,24}$/.test(value)) {
    return "Use 3 a 24 caracteres: letras, numeros ou underline.";
  }
  return null;
}

async function cropImageToSquareBlob(src: string, zoom: number, offsetX: number, offsetY: number): Promise<Blob> {
  const image = await loadImage(src);
  const canvas = document.createElement("canvas");
  canvas.width = OUTPUT_SIZE;
  canvas.height = OUTPUT_SIZE;

  const context = canvas.getContext("2d");
  if (!context) {
    throw new Error("Canvas indisponivel.");
  }

  context.fillStyle = "#111111";
  context.fillRect(0, 0, OUTPUT_SIZE, OUTPUT_SIZE);

  const baseScale = Math.max(OUTPUT_SIZE / image.naturalWidth, OUTPUT_SIZE / image.naturalHeight);
  const scale = baseScale * zoom;
  const drawWidth = image.naturalWidth * scale;
  const drawHeight = image.naturalHeight * scale;
  const maxX = Math.max(0, (drawWidth - OUTPUT_SIZE) / 2);
  const maxY = Math.max(0, (drawHeight - OUTPUT_SIZE) / 2);
  const drawX = (OUTPUT_SIZE - drawWidth) / 2 + (offsetX / 100) * maxX;
  const drawY = (OUTPUT_SIZE - drawHeight) / 2 + (offsetY / 100) * maxY;

  context.drawImage(image, drawX, drawY, drawWidth, drawHeight);

  return new Promise((resolve, reject) => {
    canvas.toBlob(
      (blob) => {
        if (!blob) {
          reject(new Error("Nao foi possivel processar a imagem."));
          return;
        }
        resolve(blob);
      },
      OUTPUT_TYPE,
      OUTPUT_QUALITY
    );
  });
}

function loadImage(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const image = new Image();
    image.onload = () => resolve(image);
    image.onerror = () => reject(new Error("Imagem invalida."));
    image.src = src;
  });
}
