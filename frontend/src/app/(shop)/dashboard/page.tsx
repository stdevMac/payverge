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
      <div className="min-h-screen bg-white relative overflow-hidden">
        {/* Subtle animated background elements */}
        <div className="absolute inset-0 overflow-hidden pointer-events-none">
          <div className="absolute -top-40 -right-40 w-80 h-80 bg-gradient-to-br from-blue-50 to-purple-50 rounded-full blur-3xl opacity-30 animate-pulse"></div>
          <div className="absolute -bottom-40 -left-40 w-96 h-96 bg-gradient-to-tr from-gray-50 to-blue-50 rounded-full blur-3xl opacity-20 animate-pulse" style={{ animationDelay: '2s' }}></div>
        </div>
        
        <div className="relative z-10 flex items-center justify-center min-h-screen">
          <div className="text-center">
            <div className="w-16 h-16 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-gray-100">
              <div className="w-8 h-8 border-2 border-gray-300 border-t-gray-900 rounded-full animate-spin"></div>
            </div>
            <p className="text-gray-600 font-light tracking-wide">Loading your businesses...</p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-white relative overflow-hidden">
      {/* Subtle animated background elements */}
      <div className="absolute inset-0 overflow-hidden pointer-events-none">
        <div className="absolute -top-40 -right-40 w-80 h-80 bg-gradient-to-br from-blue-50 to-purple-50 rounded-full blur-3xl opacity-30 animate-pulse"></div>
        <div className="absolute -bottom-40 -left-40 w-96 h-96 bg-gradient-to-tr from-gray-50 to-blue-50 rounded-full blur-3xl opacity-20 animate-pulse" style={{ animationDelay: '2s' }}></div>
      </div>

      <div className="relative z-10 max-w-6xl mx-auto px-6 py-16">
        {/* Header */}
        <section className="mb-16">
          <div className="text-center mb-8">
            <div className="inline-flex items-center gap-2 bg-gray-50 border border-gray-200 px-4 py-2 rounded-full text-sm text-gray-600 mb-6 hover:bg-gray-100 transition-colors duration-200">
              <div className="w-2 h-2 bg-green-400 rounded-full animate-pulse"></div>
              Business Dashboard
            </div>
            
            <h1 className="text-4xl lg:text-6xl font-light text-gray-900 mb-6 tracking-wide leading-tight">
              Your Businesses
            </h1>
            <p className="text-lg text-gray-600 font-light leading-relaxed max-w-2xl mx-auto mb-8">
              Manage your Payverge businesses and start accepting crypto payments
            </p>
            
            <button
              onClick={() => setShowCreateForm(true)}
              className="bg-gray-900 text-white px-8 py-3 text-base font-medium hover:bg-gray-800 transition-all duration-200 tracking-wide rounded-lg shadow-lg hover:shadow-xl"
            >
              Create Business
            </button>
          </div>
        </section>

        {/* Error Message */}
        {error && (
          <section className="mb-8">
            <div className="bg-red-50 border border-red-200 rounded-2xl p-6 shadow-sm">
              <div className="flex items-center gap-4">
                <div className="w-12 h-12 bg-red-100 rounded-xl flex items-center justify-center">
                  <svg className="h-6 w-6 text-red-600" viewBox="0 0 20 20" fill="currentColor">
                    <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" />
                  </svg>
                </div>
                <div>
                  <h3 className="text-lg font-medium text-red-900 tracking-wide">Error</h3>
                  <p className="text-red-700 font-light">{error}</p>
                </div>
              </div>
            </div>
          </section>
        )}

        {/* Businesses Grid */}
        <section>
          {businesses.length === 0 ? (
            <div className="bg-white border border-gray-200 rounded-2xl p-16 text-center shadow-sm">
              <div className="w-20 h-20 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto mb-8 border border-gray-100">
                <svg className="h-10 w-10 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4" />
                </svg>
              </div>
              <h3 className="text-2xl font-light text-gray-900 tracking-wide mb-4">No businesses yet</h3>
              <p className="text-gray-600 font-light leading-relaxed mb-8 max-w-md mx-auto">
                Get started by creating your first business and begin accepting crypto payments.
              </p>
              <button
                onClick={() => setShowCreateForm(true)}
                className="bg-gray-900 text-white px-8 py-3 text-base font-medium hover:bg-gray-800 transition-all duration-200 tracking-wide rounded-lg shadow-lg hover:shadow-xl"
              >
                Create Your First Business
              </button>
            </div>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-8">
              {businesses.map((business) => (
                <div key={business.id} className="group bg-white border border-gray-200 rounded-2xl p-8 shadow-sm hover:shadow-xl hover:border-gray-300 transition-all duration-300 hover:-translate-y-1">
                  <div className="flex items-center justify-between mb-6">
                    <h3 className="text-xl font-light text-gray-900 tracking-wide">{business.name}</h3>
                    <div className={`px-3 py-1 rounded-xl text-xs font-medium tracking-wide ${
                      business.is_active 
                        ? 'bg-green-100 text-green-700 border border-green-200' 
                        : 'bg-gray-100 text-gray-700 border border-gray-200'
                    }`}>
                      {business.is_active ? 'Active' : 'Inactive'}
                    </div>
                  </div>
                  
                  <div className="space-y-4 mb-8">
                    <div className="bg-gray-50 p-4 rounded-xl border border-gray-100">
                      <span className="text-sm font-medium text-gray-900 tracking-wide block mb-2">Settlement Address</span>
                      <span className="font-mono text-xs text-gray-600">{business.settlement_address.slice(0, 10)}...{business.settlement_address.slice(-8)}</span>
                    </div>
                    <div className="bg-gray-50 p-4 rounded-xl border border-gray-100">
                      <span className="text-sm font-medium text-gray-900 tracking-wide block mb-2">Tipping Address</span>
                      <span className="font-mono text-xs text-gray-600">{business.tipping_address.slice(0, 10)}...{business.tipping_address.slice(-8)}</span>
                    </div>
                    <div className="grid grid-cols-2 gap-4">
                      <div>
                        <span className="text-sm font-medium text-gray-900 tracking-wide block mb-1">Tax Rate</span>
                        <span className="text-gray-600 font-light">{business.tax_rate}%</span>
                      </div>
                      <div>
                        <span className="text-sm font-medium text-gray-900 tracking-wide block mb-1">Service Fee</span>
                        <span className="text-gray-600 font-light">{business.service_fee_rate}%</span>
                      </div>
                    </div>
                  </div>

                  <div className="flex gap-3">
                    <a 
                      href={`/business/${business.id}/dashboard`}
                      className="flex-1 bg-gray-900 text-white px-6 py-3 rounded-xl text-sm font-medium hover:bg-gray-800 transition-all duration-200 text-center tracking-wide group-hover:shadow-lg"
                    >
                      Manage
                    </a>
                    <a 
                      href={`/business/${business.id}/dashboard?tab=settings`}
                      className="flex-1 border border-gray-300 text-gray-700 px-6 py-3 rounded-xl text-sm font-medium hover:border-gray-400 hover:text-gray-900 transition-all duration-200 text-center tracking-wide"
                    >
                      Settings
                    </a>
                </div>
              </div>
            ))}
          </div>
        )}
        </section>

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
