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
    <main className="min-h-screen bg-neutral-950 text-white">
      <section className="mx-auto flex min-h-screen w-full max-w-5xl flex-col px-5 py-6">
        <header className="mb-6 flex items-center justify-between border-b border-white/10 pb-4">
          <div>
            <p className="text-xs uppercase tracking-[0.18em] text-neutral-500">Snooker</p>
            <h1 className="mt-1 text-2xl font-semibold tracking-normal">{isOnboarding ? "Criar perfil" : "Editar perfil"}</h1>
          </div>
          {!isOnboarding ? (
            <button className="border border-white/15 px-3 py-2 text-sm text-neutral-200 transition hover:border-white/40" onClick={() => navigate("/")}>
              Voltar
            </button>
          ) : null}
        </header>

        <div className="grid flex-1 gap-6 md:grid-cols-[320px_1fr]">
          <aside className="border border-white/10 bg-neutral-900 p-4">
            <div className="aspect-square w-full overflow-hidden border border-white/10 bg-neutral-800">
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
                <div className="flex h-full items-center justify-center text-sm text-neutral-500">1:1</div>
              )}
            </div>

            <div className="mt-4 border-t border-white/10 pt-4">
              <p className="text-sm font-medium text-white">{nickname || "nickname"}</p>
              <p className="mt-1 min-h-10 text-sm text-neutral-400">{bio || "bio"}</p>
              <div className="mt-4 border border-white/10 bg-white/5 p-3">
                <div className="flex items-center justify-between text-sm">
                  <span className="font-semibold text-white">Nivel {level}</span>
                  <span className="text-neutral-400">{totalXP} XP</span>
                </div>
                <div className="mt-3 h-2 bg-neutral-800">
                  <div className="h-full bg-emerald-400" style={{ width: `${levelProgress}%` }} />
                </div>
                <p className="mt-2 text-xs text-neutral-500">{nextLevelXP - totalXP} XP para o proximo nivel</p>
              </div>
            </div>
          </aside>

          <form className="border border-white/10 bg-white p-5 text-black" onSubmit={handleSubmit}>
            {loading ? (
              <div className="h-80 animate-pulse bg-neutral-100" />
            ) : (
              <div className="space-y-5">
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

                <label className="block text-sm text-neutral-800" htmlFor="bio">
                  <span className="mb-2 block">Bio</span>
                  <textarea
                    id="bio"
                    name="bio"
                    className="min-h-28 w-full resize-none border border-neutral-300 bg-white px-3 py-2 text-sm text-black outline-none transition placeholder:text-neutral-400 focus:border-black"
                    maxLength={200}
                    value={bio}
                    onChange={(event) => setBio(event.target.value)}
                  />
                </label>
                <div className="-mt-4 flex justify-between text-xs text-neutral-500">
                  <span>{bioIssue ?? ""}</span>
                  <span>{bio.length}/200</span>
                </div>

                <label className="block text-sm text-neutral-800" htmlFor="photo">
                  <span className="mb-2 block">Foto</span>
                  <input
                    id="photo"
                    name="photo"
                    type="file"
                    accept="image/jpeg,image/png,image/webp"
                    className="block w-full border border-neutral-300 bg-white text-sm text-neutral-700 file:mr-4 file:h-10 file:border-0 file:bg-black file:px-4 file:text-sm file:font-medium file:text-white"
                    onChange={handleFile}
                  />
                </label>
                {imageName ? <p className="-mt-3 text-xs text-neutral-500">{imageName}</p> : null}

                {imageUrl ? (
                  <div className="grid gap-4 border border-neutral-200 bg-neutral-50 p-4 md:grid-cols-3">
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
    <label className="block text-xs font-medium uppercase tracking-[0.14em] text-neutral-500">
      <span className="mb-2 block">{label}</span>
      <input
        className="w-full accent-black"
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
