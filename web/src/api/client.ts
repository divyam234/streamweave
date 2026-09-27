import createClient from 'openapi-fetch'
import type { paths } from './schema'

const csrfHeader = 'X-CSRF-Protection'

const authenticatedFetch: typeof fetch = async (input, init) => {
  const method = (init?.method ?? (input instanceof Request ? input.method : 'GET')).toUpperCase()
  const headers = new Headers(init?.headers)

  if (input instanceof Request) {
    input.headers.forEach((value, key) => {
      if (!headers.has(key)) headers.set(key, value)
    })
  }

  if (!['GET', 'HEAD', 'OPTIONS'].includes(method)) {
    headers.set(csrfHeader, '1')
  }

  const response = await fetch(input, {
    ...init,
    credentials: 'same-origin',
    headers,
  })
  if (response.status === 401 && typeof window !== 'undefined') {
    window.dispatchEvent(new Event('media-engine-auth-expired'))
  }
  return response
}

export async function getAdminSession() {
  const response = await fetch('/auth/session', {
    credentials: 'same-origin',
    headers: { Accept: 'application/json' },
  })
  return response.ok
}

export async function loginAdmin(token: string) {
  const response = await fetch('/auth/login', {
    method: 'POST',
    credentials: 'same-origin',
    headers: {
      'Content-Type': 'application/json',
      [csrfHeader]: '1',
    },
    body: JSON.stringify({ token }),
  })
  if (!response.ok) {
    throw new Error('Invalid admin token')
  }
}

export async function logoutAdmin() {
  const response = await fetch('/auth/logout', {
    method: 'POST',
    credentials: 'same-origin',
    headers: { [csrfHeader]: '1' },
  })
  if (!response.ok) {
    throw new Error('Unable to log out')
  }
}

export const api = createClient<paths>({
  baseUrl: '',
  fetch: authenticatedFetch,
})
