'use client'

import { useParams, useRouter } from 'next/navigation'
import { useEffect, useState } from 'react'
import { Button, Spinner } from '@nextui-org/react'
import { useUserStore } from '@/store/useUserStore'
import { useAuth } from '@/hooks/useAuth'
import { getCookie } from '@/config/aws-s3/cookie-management/store.helpers'
import { isTokenValid, decodeJwt } from '@/utils/jwt'
import { getUserProfile } from '@/api/users/profile'
import { UserInterface as User } from '@/interface/users/users-interface'
import Dashboard from '@/components/dashboard/Dashboard'

export default function AnalyticsPage() {
  const params = useParams()
  const router = useRouter()
  const businessId = params.businessId as string
  
  const { user } = useUserStore()
  const { isAuthenticated } = useAuth()
  const [authLoading, setAuthLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // Authentication check
  useEffect(() => {
    const checkAuth = async () => {
      try {
        const token = getCookie("session_token")
        
        if (!token || !isTokenValid(token)) {
          // Wait a moment to avoid flashing error states
          setTimeout(() => {
            setError("Please sign in to continue.")
            setAuthLoading(false)
          }, 1000)
          return
        }

        const tokenData = decodeJwt(token)
        
        if (!tokenData.address) {
          setTimeout(() => {
            setError("Invalid session token.")
            setAuthLoading(false)
          }, 1000)
          return
        }

        const address = tokenData.address.toLowerCase()
        
        if (!user) {
          // Try to fetch user data if not in store
          try {
            const userData = await getUserProfile(address)
            if (userData) {
              userData.role = tokenData.role || 'user'
              useUserStore.getState().setUser(userData)
            } else {
              // Create temporary user from token
              const tempUser: User = {
                username: "",
                email: "",
                address: address,
                role: tokenData.role || 'user',
                joined_at: new Date().toISOString(),
                language_selected: "en",
                referral_code: "",
                referrer: "",
                referees: {},
                notifications: [],
              }
              useUserStore.getState().setUser(tempUser)
            }
          } catch (apiError) {
            // If API fails, still create temp user from token
            const tempUser: User = {
              username: "",
              email: "",
              address: address,
              role: tokenData.role || 'user',
              joined_at: new Date().toISOString(),
              language_selected: "en",
              referral_code: "",
              referrer: "",
              referees: {},
              notifications: [],
            }
            useUserStore.getState().setUser(tempUser)
          }
        }
        
        setAuthLoading(false)
      } catch (error) {
        setTimeout(() => {
          setError("Authentication failed. Please try signing in again.")
          setAuthLoading(false)
        }, 1000)
      }
    }

    checkAuth()
  }, [user])

  // Show loading screen while checking authentication
  if (authLoading) {
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
            <p className="text-gray-600 font-light tracking-wide">Loading analytics...</p>
          </div>
        </div>
      </div>
    )
  }

  // Show authentication error
  if (error && (error.includes("sign in") || error.includes("Authentication failed"))) {
    return (
      <div className="min-h-screen bg-white relative overflow-hidden">
        {/* Subtle animated background elements */}
        <div className="absolute inset-0 overflow-hidden pointer-events-none">
          <div className="absolute -top-40 -right-40 w-80 h-80 bg-gradient-to-br from-blue-50 to-purple-50 rounded-full blur-3xl opacity-30 animate-pulse"></div>
          <div className="absolute -bottom-40 -left-40 w-96 h-96 bg-gradient-to-tr from-gray-50 to-blue-50 rounded-full blur-3xl opacity-20 animate-pulse" style={{ animationDelay: '2s' }}></div>
        </div>
        
        <div className="relative z-10 flex items-center justify-center min-h-screen">
          <div className="text-center max-w-md mx-auto">
            <div className="w-16 h-16 bg-red-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-red-100">
              <svg className="w-8 h-8 text-red-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
              </svg>
            </div>
            <h2 className="text-2xl font-light text-gray-900 mb-4 tracking-wide">Authentication Required</h2>
            <p className="text-gray-600 font-light tracking-wide mb-8">{error}</p>
            
            <div className="space-y-4">
              <p className="text-sm text-gray-500">Please authenticate to view business analytics.</p>
              <button
                onClick={() => window.location.reload()}
                className="px-6 py-3 text-sm font-medium text-white bg-gray-900 border border-transparent rounded-md hover:bg-gray-800 transition-colors duration-200"
              >
                Try Again
              </button>
            </div>
            
            <div className="mt-8">
              <button 
                onClick={() => router.push('/dashboard')}
                className="text-sm text-gray-500 hover:text-gray-700 transition-colors duration-200"
              >
                ← Back to Dashboard
              </button>
            </div>
          </div>
        </div>
      </div>
    )
  }

  return <Dashboard businessId={businessId} />
}
