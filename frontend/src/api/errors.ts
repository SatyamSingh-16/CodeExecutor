export class ApiError extends Error {
  public readonly status: number;
  public readonly details?: Record<string, string>;

  constructor(status: number, message: string, details?: Record<string, string>) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.details = details;
    Object.setPrototypeOf(this, ApiError.prototype);
  }

  public isNotFound(): boolean {
    return this.status === 404;
  }

  public isUnauthorized(): boolean {
    return this.status === 401;
  }

  public isRateLimited(): boolean {
    return this.status === 429;
  }
}
