// Business API utilities
export interface BusinessAddress {
  street: string;
  city: string;
  state: string;
  postal_code: string;
  country: string;
}

export interface Business {
  id: number;
  owner_address: string;
  name: string;
  logo: string;
  address: BusinessAddress;
  settlement_address: string;
  tipping_address: string;
  tax_rate: number;
  service_fee_rate: number;
  tax_inclusive: boolean;
  service_inclusive: boolean;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface CreateBusinessRequest {
  name: string;
  logo?: string;
  address: BusinessAddress;
  settlement_address: string;
  tipping_address: string;
  tax_rate: number;
  service_fee_rate: number;
  tax_inclusive: boolean;
  service_inclusive: boolean;
}

export interface UpdateBusinessRequest {
  name?: string;
  logo?: string;
  address?: BusinessAddress;
  settlement_address?: string;
  tipping_address?: string;
  tax_rate?: number;
  service_fee_rate?: number;
  tax_inclusive?: boolean;
  service_inclusive?: boolean;
}

const API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';

// Get JWT token from localStorage
const getAuthToken = () => {
  if (typeof window !== 'undefined') {
    return localStorage.getItem('jwt_token');
  }
  return null;
};

// Create headers with authentication
const getAuthHeaders = () => {
  const token = getAuthToken();
  return {
    'Content-Type': 'application/json',
    ...(token && { 'Authorization': `Bearer ${token}` }),
  };
};

// Create a new business
export const createBusiness = async (businessData: CreateBusinessRequest): Promise<Business> => {
  const response = await fetch(`${API_BASE}/inside/businesses`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify(businessData),
  });

  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error || 'Failed to create business');
  }

  return response.json();
};

// Get all businesses owned by the authenticated user
export const getMyBusinesses = async (): Promise<Business[]> => {
  const response = await fetch(`${API_BASE}/inside/businesses`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });

  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error || 'Failed to fetch businesses');
  }

  return response.json();
};

// Get a specific business by ID
export const getBusiness = async (businessId: number): Promise<Business> => {
  const response = await fetch(`${API_BASE}/inside/businesses/${businessId}`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });

  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error || 'Failed to fetch business');
  }

  return response.json();
};

// Update a business
export const updateBusiness = async (businessId: number, businessData: UpdateBusinessRequest): Promise<Business> => {
  const response = await fetch(`${API_BASE}/inside/businesses/${businessId}`, {
    method: 'PUT',
    headers: getAuthHeaders(),
    body: JSON.stringify(businessData),
  });

  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error || 'Failed to update business');
  }

  return response.json();
};

// Delete a business
export const deleteBusiness = async (businessId: number): Promise<void> => {
  const response = await fetch(`${API_BASE}/inside/businesses/${businessId}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });

  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error || 'Failed to delete business');
  }
};

// Menu Management Types
export interface MenuItem {
  name: string;
  description: string;
  price: number;
  image?: string;
  isAvailable: boolean;
}

export interface MenuCategory {
  name: string;
  description: string;
  items: MenuItem[];
}

export interface Menu {
  id: number;
  business_id: number;
  categories: MenuCategory[];
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

// Phase 2: Enhanced Menu Management API Functions

// Get menu for a business
export const getMenu = async (businessId: number): Promise<Menu> => {
  const response = await fetch(`${API_BASE}/inside/businesses/${businessId}/menu`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });

  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error || 'Failed to fetch menu');
  }

  return response.json();
};

// Add a new category to menu
export const addMenuCategory = async (businessId: number, category: MenuCategory): Promise<void> => {
  const response = await fetch(`${API_BASE}/inside/businesses/${businessId}/menu/categories`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify(category),
  });

  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error || 'Failed to add category');
  }
};

// Update a menu category
export const updateMenuCategory = async (businessId: number, categoryIndex: number, category: MenuCategory): Promise<void> => {
  const response = await fetch(`${API_BASE}/inside/businesses/${businessId}/menu/categories/${categoryIndex}`, {
    method: 'PUT',
    headers: getAuthHeaders(),
    body: JSON.stringify(category),
  });

  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error || 'Failed to update category');
  }
};

// Delete a menu category
export const deleteMenuCategory = async (businessId: number, categoryIndex: number): Promise<void> => {
  const response = await fetch(`${API_BASE}/inside/businesses/${businessId}/menu/categories/${categoryIndex}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });

  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error || 'Failed to delete category');
  }
};

// Add a menu item to a category
export const addMenuItem = async (businessId: number, categoryIndex: number, item: MenuItem): Promise<void> => {
  const response = await fetch(`${API_BASE}/inside/businesses/${businessId}/menu/items`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({
      category_index: categoryIndex,
      item: item,
    }),
  });

  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error || 'Failed to add menu item');
  }
};

// Update a menu item
export const updateMenuItem = async (businessId: number, categoryIndex: number, itemIndex: number, item: MenuItem): Promise<void> => {
  const response = await fetch(`${API_BASE}/inside/businesses/${businessId}/menu/items`, {
    method: 'PUT',
    headers: getAuthHeaders(),
    body: JSON.stringify({
      category_index: categoryIndex,
      item_index: itemIndex,
      item: item,
    }),
  });

  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error || 'Failed to update menu item');
  }
};

// Delete a menu item
export const deleteMenuItem = async (businessId: number, categoryIndex: number, itemIndex: number): Promise<void> => {
  const response = await fetch(`${API_BASE}/inside/businesses/${businessId}/menu/categories/${categoryIndex}/items/${itemIndex}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });

  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error || 'Failed to delete menu item');
  }
};

// Table Management Types
export interface Table {
  id: number;
  business_id: number;
  name: string;
  table_code: string;
  qr_url: string;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface CreateTableRequest {
  name: string;
}

export interface UpdateTableRequest {
  name?: string;
  is_active?: boolean;
}

// Phase 2: Enhanced Table Management API Functions

// Get all tables for a business
export const getBusinessTables = async (businessId: number): Promise<{ tables: Table[] }> => {
  const response = await fetch(`${API_BASE}/inside/businesses/${businessId}/tables`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });

  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error || 'Failed to fetch tables');
  }

  return response.json();
};

// Create a new table with QR code
export const createTableWithQR = async (businessId: number, tableData: CreateTableRequest): Promise<Table> => {
  const response = await fetch(`${API_BASE}/inside/businesses/${businessId}/tables`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify(tableData),
  });

  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error || 'Failed to create table');
  }

  return response.json();
};

// Update table details
export const updateTableDetails = async (tableId: number, tableData: UpdateTableRequest): Promise<Table> => {
  const response = await fetch(`${API_BASE}/inside/tables/${tableId}`, {
    method: 'PUT',
    headers: getAuthHeaders(),
    body: JSON.stringify(tableData),
  });

  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error || 'Failed to update table');
  }

  return response.json();
};

// Delete a table (soft delete)
export const deleteTable = async (tableId: number): Promise<void> => {
  const response = await fetch(`${API_BASE}/inside/tables/${tableId}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });

  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error || 'Failed to delete table');
  }
};

// Export all functions as businessApi object
export const businessApi = {
  createBusiness,
  getMyBusinesses,
  getBusiness,
  updateBusiness,
  deleteBusiness,
  getMenu,
  addMenuCategory,
  updateMenuCategory,
  deleteMenuCategory,
  addMenuItem,
  updateMenuItem,
  deleteMenuItem,
  getBusinessTables,
  createTableWithQR,
  updateTableDetails,
  deleteTable,
};
