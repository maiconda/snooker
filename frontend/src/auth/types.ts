export type FieldError = {
  field: string;
  issue: string;
};

export type AccountStatus = "onboarding_pending" | "active" | "blocked";

export type AuthSession = {
  accessToken: string;
  status: AccountStatus;
  email?: string;
  expiresAt?: number;
};

export type LoginPayload = {
  email: string;
  password: string;
};

export type SignupPayload = {
  email: string;
  password: string;
};

export type AuthResponse = {
  token: string;
  status: AccountStatus;
  message?: string;
};

export type RefreshResponse = {
  access_token: string;
};

export type ApiErrorBody = {
  error?: {
    code?: string;
    message?: string;
    details?: FieldError[];
  };
};

export class AuthApiError extends Error {
  code: string;
  details: FieldError[];
  status: number;

  constructor(message: string, code: string, status: number, details: FieldError[] = []) {
    super(message);
    this.name = "AuthApiError";
    this.code = code;
    this.status = status;
    this.details = details;
  }
}
