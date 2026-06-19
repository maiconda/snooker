export type Profile = {
  user_id: string;
  nickname: string;
  bio: string;
  photo_url: string;
  xp: number;
  created_at: string;
  updated_at: string;
};

export type PhotoUploadURLResponse = {
  upload_id: string;
  upload_url: string;
  object_key: string;
  expires_at: string;
  public_url: string;
  max_size: number;
};

export type CompleteProfileResponse = {
  profile: Profile;
  access_token: string;
  status: "active";
};

export type ProfilePayload = {
  nickname: string;
  bio: string;
  photo_upload_id?: string;
};

export type UpdateProfilePayload = {
  nickname?: string;
  bio?: string;
  photo_upload_id?: string;
};
