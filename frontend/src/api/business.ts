import { axiosInstance } from './index';

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

// Create a new business
export const createBusiness = async (businessData: CreateBusinessRequest): Promise<Business> => {
  const response = await axiosInstance.post<Business>('/inside/businesses', businessData);
  return response.data;
};

// Get all businesses owned by the authenticated user
export const getMyBusinesses = async (): Promise<Business[]> => {
  const response = await axiosInstance.get<Business[]>('/inside/businesses');
  return response.data;
};

// Get a specific business by ID
export const getBusiness = async (businessId: number): Promise<Business> => {
  const response = await axiosInstance.get<Business>(`/inside/businesses/${businessId}`);
  return response.data;
};

// Update a business
export const updateBusiness = async (businessId: number, businessData: UpdateBusinessRequest): Promise<Business> => {
  const response = await axiosInstance.put<Business>(`/inside/businesses/${businessId}`, businessData);
  return response.data;
};

// Delete a business
export const deleteBusiness = async (businessId: number): Promise<void> => {
  await axiosInstance.delete(`/inside/businesses/${businessId}`);
};

// Menu Management Types
export interface MenuItemOption {
  id?: string;
  name: string;
  price_change: number;
  is_required?: boolean;
}

export interface MenuItem {
  id?: string;
  name: string;
  description: string;
  price: number;
  currency?: string;
  image?: string;        // Keep for backward compatibility
  images?: string[];     // New field for multiple images
  options?: MenuItemOption[];
  allergens?: string[];
  dietary_tags?: string[];
  is_available: boolean;
  sort_order?: number;
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
  const response = await axiosInstance.get<Menu>(`/inside/businesses/${businessId}/menu`);
  return response.data;
};

// Add a new category to menu
export const addMenuCategory = async (businessId: number, category: MenuCategory): Promise<void> => {
  await axiosInstance.post(`/inside/businesses/${businessId}/menu/categories`, category);
};

// Update a menu category
export const updateMenuCategory = async (businessId: number, categoryIndex: number, category: MenuCategory): Promise<void> => {
  await axiosInstance.put(`/inside/businesses/${businessId}/menu/categories/${categoryIndex}`, category);
};

// Delete a menu category
export const deleteMenuCategory = async (businessId: number, categoryIndex: number): Promise<void> => {
  await axiosInstance.delete(`/inside/businesses/${businessId}/menu/categories/${categoryIndex}`);
};

// Add a menu item to a category
export const addMenuItem = async (businessId: number, categoryIndex: number, item: MenuItem): Promise<void> => {
  // Transform the item to match backend expectations
  const backendItem = {
    id: item.id || '', // Backend will generate ID if empty
    name: item.name,
    description: item.description,
    price: item.price,
    currency: item.currency || 'USD',
    image: item.image || '',
    images: item.images || [],
    options: item.options || [],
    allergens: item.allergens || [],
    dietary_tags: item.dietary_tags || [],
    is_available: item.is_available,
    sort_order: item.sort_order || 0,
  };

  await axiosInstance.post(`/inside/businesses/${businessId}/menu/items`, {
    category_index: categoryIndex,
    item: backendItem,
  });
};

// Update a menu item
export const updateMenuItem = async (businessId: number, categoryIndex: number, itemIndex: number, item: MenuItem): Promise<void> => {
  // Transform the item to match backend expectations
  const backendItem = {
    id: item.id || '', // Backend will handle ID
    name: item.name,
    description: item.description,
    price: item.price,
    currency: item.currency || 'USD',
    image: item.image || '',
    images: item.images || [],
    options: item.options || [],
    allergens: item.allergens || [],
    dietary_tags: item.dietary_tags || [],
    is_available: item.is_available,
    sort_order: item.sort_order || 0,
  };

  await axiosInstance.put(`/inside/businesses/${businessId}/menu/items`, {
    category_index: categoryIndex,
    item_index: itemIndex,
    item: backendItem,
  });
};

// Delete a menu item
export const deleteMenuItem = async (businessId: number, categoryIndex: number, itemIndex: number): Promise<void> => {
  await axiosInstance.delete(`/inside/businesses/${businessId}/menu/categories/${categoryIndex}/items/${itemIndex}`);
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
  const response = await axiosInstance.get<{ tables: Table[] }>(`/inside/businesses/${businessId}/tables`);
  return response.data;
};

// Create a new table with QR code
export const createTableWithQR = async (businessId: number, tableData: CreateTableRequest): Promise<Table> => {
  const response = await axiosInstance.post<Table>(`/inside/businesses/${businessId}/tables`, tableData);
  return response.data;
};

// Update table details
export const updateTableDetails = async (tableId: number, tableData: UpdateTableRequest): Promise<Table> => {
  const response = await axiosInstance.put<Table>(`/inside/tables/${tableId}`, tableData);
  return response.data;
};

// Delete a table (soft delete)
export const deleteTable = async (tableId: number): Promise<void> => {
  await axiosInstance.delete(`/inside/tables/${tableId}`);
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
