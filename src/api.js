const API_URL = '/api';

export const api = {
  // Auth
  register: async (email, password) => {
    const res = await fetch(`${API_URL}/auth/register`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email, password })
    });
    if (!res.ok) throw new Error('Registration failed');
    return res.json();
  },

  login: async (email, password) => {
    const res = await fetch(`${API_URL}/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email, password })
    });
    if (!res.ok) throw new Error('Login failed');
    return res.json();
  },

  // Tunnels
  getTunnels: async (token) => {
    const res = await fetch(`${API_URL}/tunnels`, {
      headers: { 'Authorization': `Bearer ${token}` }
    });
    return res.json();
  },

  reserveTunnel: async (token, subdomain) => {
    const res = await fetch(`${API_URL}/tunnels`, {
      method: 'POST',
      headers: { 'Authorization': `Bearer ${token}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({ subdomain })
    });
    return res.json();
  },

  // Request Inspector
  getRequestLogs: async (token, subdomain, limit = 50, offset = 0) => {
    const res = await fetch(`${API_URL}/requests/${subdomain}?limit=${limit}&offset=${offset}`, {
      headers: { 'Authorization': `Bearer ${token}` }
    });
    if (!res.ok) throw new Error('Failed to fetch request logs');
    return res.json();
  },

  getRequestLog: async (token, subdomain, requestId) => {
    const res = await fetch(`${API_URL}/requests/${subdomain}/${requestId}`, {
      headers: { 'Authorization': `Bearer ${token}` }
    });
    if (!res.ok) throw new Error('Failed to fetch request');
    return res.json();
  },

  // Organizations
  getOrganizations: async (token) => {
    const res = await fetch(`${API_URL}/orgs`, {
      headers: { 'Authorization': `Bearer ${token}` }
    });
    return res.json();
  },

  createOrganization: async (token, name, slug) => {
    const res = await fetch(`${API_URL}/orgs`, {
      method: 'POST',
      headers: { 'Authorization': `Bearer ${token}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, slug })
    });
    if (!res.ok) throw new Error('Failed to create organization');
    return res.json();
  },

  getOrganizationMembers: async (token, orgId) => {
    const res = await fetch(`${API_URL}/orgs/${orgId}/members`, {
      headers: { 'Authorization': `Bearer ${token}` }
    });
    return res.json();
  }
};