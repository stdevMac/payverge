"use client";
import { useState, useEffect } from "react";
import { Business, getMyBusinesses, createBusiness, CreateBusinessRequest } from "@/api/business";

export default function Dashboard() {
  const [businesses, setBusinesses] = useState<Business[]>([]);
  const [loading, setLoading] = useState(true);
  const [showCreateForm, setShowCreateForm] = useState(false);
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Form state
  const [formData, setFormData] = useState<CreateBusinessRequest>({
    name: "",
    logo: "",
    address: {
      street: "",
      city: "",
      state: "",
      postal_code: "",
      country: "",
    },
    settlement_address: "",
    tipping_address: "",
    tax_rate: 0,
    service_fee_rate: 0,
    tax_inclusive: false,
    service_inclusive: false,
  });

  useEffect(() => {
    loadBusinesses();
  }, []);

  const loadBusinesses = async () => {
    try {
      setLoading(true);
      const data = await getMyBusinesses();
      setBusinesses(data);
    } catch (err: any) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const handleCreateBusiness = async (e: React.FormEvent) => {
    e.preventDefault();
    setCreating(true);
    setError(null);

    try {
      const newBusiness = await createBusiness(formData);
      setBusinesses([...businesses, newBusiness]);
      setShowCreateForm(false);
      setFormData({
        name: "",
        logo: "",
        address: {
          street: "",
          city: "",
          state: "",
          postal_code: "",
          country: "",
        },
        settlement_address: "",
        tipping_address: "",
        tax_rate: 0,
        service_fee_rate: 0,
        tax_inclusive: false,
        service_inclusive: false,
      });
    } catch (err: any) {
      setError(err.message);
    } finally {
      setCreating(false);
    }
  };

  if (loading) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center">
        <div className="text-center">
          <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-gray-900 mx-auto mb-4"></div>
          <p className="text-gray-600">Loading your businesses...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gray-50">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
        {/* Header */}
        <div className="mb-8">
          <div className="flex items-center justify-between">
            <div>
              <h1 className="text-3xl font-bold text-gray-900">Business Dashboard</h1>
              <p className="text-gray-600 mt-2">Manage your Payverge businesses</p>
            </div>
            <button
              onClick={() => setShowCreateForm(true)}
              className="bg-gray-900 text-white px-6 py-3 rounded-lg hover:bg-gray-800 transition-colors font-medium"
            >
              Create Business
            </button>
          </div>
        </div>

        {/* Error Message */}
        {error && (
          <div className="mb-6 bg-red-50 border border-red-200 rounded-lg p-4">
            <div className="flex">
              <div className="flex-shrink-0">
                <svg className="h-5 w-5 text-red-400" viewBox="0 0 20 20" fill="currentColor">
                  <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" />
                </svg>
              </div>
              <div className="ml-3">
                <p className="text-sm text-red-800">{error}</p>
              </div>
            </div>
          </div>
        )}

        {/* Businesses Grid */}
        {businesses.length === 0 ? (
          <div className="text-center py-12">
            <svg className="mx-auto h-12 w-12 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4" />
            </svg>
            <h3 className="mt-2 text-sm font-medium text-gray-900">No businesses</h3>
            <p className="mt-1 text-sm text-gray-500">Get started by creating your first business.</p>
            <div className="mt-6">
              <button
                onClick={() => setShowCreateForm(true)}
                className="inline-flex items-center px-4 py-2 border border-transparent shadow-sm text-sm font-medium rounded-md text-white bg-gray-900 hover:bg-gray-800"
              >
                Create Business
              </button>
            </div>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
            {businesses.map((business) => (
              <div key={business.id} className="bg-white rounded-lg shadow-sm border border-gray-200 p-6 hover:shadow-md transition-shadow">
                <div className="flex items-center justify-between mb-4">
                  <h3 className="text-lg font-semibold text-gray-900">{business.name}</h3>
                  <div className={`px-2 py-1 rounded-full text-xs font-medium ${
                    business.is_active 
                      ? 'bg-green-100 text-green-800' 
                      : 'bg-gray-100 text-gray-800'
                  }`}>
                    {business.is_active ? 'Active' : 'Inactive'}
                  </div>
                </div>
                
                <div className="space-y-2 text-sm text-gray-600">
                  <div>
                    <span className="font-medium">Settlement:</span>
                    <span className="ml-1 font-mono text-xs">{business.settlement_address.slice(0, 10)}...{business.settlement_address.slice(-8)}</span>
                  </div>
                  <div>
                    <span className="font-medium">Tipping:</span>
                    <span className="ml-1 font-mono text-xs">{business.tipping_address.slice(0, 10)}...{business.tipping_address.slice(-8)}</span>
                  </div>
                  <div>
                    <span className="font-medium">Tax Rate:</span>
                    <span className="ml-1">{business.tax_rate}%</span>
                  </div>
                  <div>
                    <span className="font-medium">Service Fee:</span>
                    <span className="ml-1">{business.service_fee_rate}%</span>
                  </div>
                </div>

                <div className="mt-6 flex space-x-3">
                  <a 
                    href={`/business/${business.id}/dashboard`}
                    className="flex-1 bg-gray-100 text-gray-700 px-4 py-2 rounded-md text-sm font-medium hover:bg-gray-200 transition-colors text-center"
                  >
                    Manage
                  </a>
                  <a 
                    href={`/business/${business.id}/dashboard?tab=settings`}
                    className="flex-1 border border-gray-300 text-gray-700 px-4 py-2 rounded-md text-sm font-medium hover:bg-gray-50 transition-colors text-center"
                  >
                    Settings
                  </a>
                </div>
              </div>
            ))}
          </div>
        )}

        {/* Create Business Modal */}
        {showCreateForm && (
          <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
            <div className="bg-white rounded-lg max-w-2xl w-full max-h-[90vh] overflow-y-auto">
              <div className="p-6">
                <div className="flex items-center justify-between mb-6">
                  <h2 className="text-xl font-semibold text-gray-900">Create New Business</h2>
                  <button
                    onClick={() => setShowCreateForm(false)}
                    className="text-gray-400 hover:text-gray-600"
                  >
                    <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                    </svg>
                  </button>
                </div>

                <form onSubmit={handleCreateBusiness} className="space-y-6">
                  {/* Basic Information */}
                  <div>
                    <h3 className="text-lg font-medium text-gray-900 mb-4">Basic Information</h3>
                    <div className="grid grid-cols-1 gap-4">
                      <div>
                        <label className="block text-sm font-medium text-gray-700 mb-2">
                          Business Name *
                        </label>
                        <input
                          type="text"
                          required
                          value={formData.name}
                          onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                          className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-1 focus:ring-gray-500"
                          placeholder="Enter business name"
                        />
                      </div>
                    </div>
                  </div>

                  {/* Wallet Addresses */}
                  <div>
                    <h3 className="text-lg font-medium text-gray-900 mb-4">Wallet Configuration</h3>
                    <div className="grid grid-cols-1 gap-4">
                      <div>
                        <label className="block text-sm font-medium text-gray-700 mb-2">
                          Settlement Address *
                        </label>
                        <input
                          type="text"
                          required
                          value={formData.settlement_address}
                          onChange={(e) => setFormData({ ...formData, settlement_address: e.target.value })}
                          className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-1 focus:ring-gray-500 font-mono text-sm"
                          placeholder="0x..."
                        />
                        <p className="text-xs text-gray-500 mt-1">Where business payments will be sent</p>
                      </div>
                      <div>
                        <label className="block text-sm font-medium text-gray-700 mb-2">
                          Tipping Address *
                        </label>
                        <input
                          type="text"
                          required
                          value={formData.tipping_address}
                          onChange={(e) => setFormData({ ...formData, tipping_address: e.target.value })}
                          className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-1 focus:ring-gray-500 font-mono text-sm"
                          placeholder="0x..."
                        />
                        <p className="text-xs text-gray-500 mt-1">Where tips will be sent</p>
                      </div>
                    </div>
                  </div>

                  {/* Tax & Service Fees */}
                  <div>
                    <h3 className="text-lg font-medium text-gray-900 mb-4">Tax & Service Fees</h3>
                    <div className="grid grid-cols-2 gap-4">
                      <div>
                        <label className="block text-sm font-medium text-gray-700 mb-2">
                          Tax Rate (%)
                        </label>
                        <input
                          type="number"
                          step="0.01"
                          min="0"
                          max="100"
                          value={formData.tax_rate}
                          onChange={(e) => setFormData({ ...formData, tax_rate: parseFloat(e.target.value) || 0 })}
                          className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-1 focus:ring-gray-500"
                        />
                      </div>
                      <div>
                        <label className="block text-sm font-medium text-gray-700 mb-2">
                          Service Fee (%)
                        </label>
                        <input
                          type="number"
                          step="0.01"
                          min="0"
                          max="100"
                          value={formData.service_fee_rate}
                          onChange={(e) => setFormData({ ...formData, service_fee_rate: parseFloat(e.target.value) || 0 })}
                          className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-1 focus:ring-gray-500"
                        />
                      </div>
                    </div>
                    <div className="mt-4 space-y-2">
                      <label className="flex items-center">
                        <input
                          type="checkbox"
                          checked={formData.tax_inclusive}
                          onChange={(e) => setFormData({ ...formData, tax_inclusive: e.target.checked })}
                          className="rounded border-gray-300 text-gray-600 focus:ring-gray-500"
                        />
                        <span className="ml-2 text-sm text-gray-700">Tax inclusive pricing</span>
                      </label>
                      <label className="flex items-center">
                        <input
                          type="checkbox"
                          checked={formData.service_inclusive}
                          onChange={(e) => setFormData({ ...formData, service_inclusive: e.target.checked })}
                          className="rounded border-gray-300 text-gray-600 focus:ring-gray-500"
                        />
                        <span className="ml-2 text-sm text-gray-700">Service fee inclusive pricing</span>
                      </label>
                    </div>
                  </div>

                  {/* Form Actions */}
                  <div className="flex justify-end space-x-4 pt-6 border-t">
                    <button
                      type="button"
                      onClick={() => setShowCreateForm(false)}
                      className="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-md hover:bg-gray-50"
                    >
                      Cancel
                    </button>
                    <button
                      type="submit"
                      disabled={creating}
                      className="px-4 py-2 text-sm font-medium text-white bg-gray-900 border border-transparent rounded-md hover:bg-gray-800 disabled:opacity-50 disabled:cursor-not-allowed"
                    >
                      {creating ? "Creating..." : "Create Business"}
                    </button>
                  </div>
                </form>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
