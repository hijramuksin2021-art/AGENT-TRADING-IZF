import { useState } from 'react'
import HeaderBar from '../components/common/HeaderBar'
import LoginModal from '../components/landing/LoginModal'
import { LoginRequiredOverlay } from '../components/auth/LoginRequiredOverlay'
import FooterSection from '../components/landing/FooterSection'
import TerminalHero from '../components/landing/core/TerminalHero'
import LiveFeed from '../components/landing/core/LiveFeed'
import AgentGrid from '../components/landing/core/AgentGrid'
import DeploymentHub from '../components/landing/core/DeploymentHub'
import { useAuth } from '../contexts/AuthContext'
import { useLanguage } from '../contexts/LanguageContext'

export function LandingPage() {
  const [showLoginModal, setShowLoginModal] = useState(false)
  const [loginOverlayOpen, setLoginOverlayOpen] = useState(false)
  const [loginOverlayFeature, setLoginOverlayFeature] = useState('')
  const { user, logout } = useAuth()
  const { language, setLanguage } = useLanguage()
  const isLoggedIn = !!user

  const handleLoginRequired = (featureName: string) => {
    setLoginOverlayFeature(featureName)
    setLoginOverlayOpen(true)
  }

  return (
    <>
      <HeaderBar
        onLoginClick={() => setShowLoginModal(true)}
        isLoggedIn={isLoggedIn}
        isHomePage={true}
        language={language}
        onLanguageChange={setLanguage}
        user={user}
        onLogout={logout}
        onLoginRequired={handleLoginRequired}
      />
      <div className="min-h-screen bg-nofx-bg text-nofx-text font-sans selection:bg-nofx-gold selection:text-black">
        <TerminalHero />

        <LiveFeed />

        <AgentGrid />

        <DeploymentHub />

        <FooterSection language={language} />

        {showLoginModal && (
          <LoginModal
            onClose={() => setShowLoginModal(false)}
            language={language}
          />
        )}

        <LoginRequiredOverlay
          isOpen={loginOverlayOpen}
          onClose={() => setLoginOverlayOpen(false)}
          featureName={loginOverlayFeature}
        />
      </div>

      {/* Fixed "Built by" Badge - pojok kanan bawah */}
      <a
        href="https://github.com/hijramuksin2021-art/AGENT-TRADING-IZF"
        target="_blank"
        rel="noopener noreferrer"
        style={{
          position: 'fixed',
          bottom: '16px',
          right: '16px',
          zIndex: 9999,
          display: 'flex',
          alignItems: 'center',
          gap: '6px',
          padding: '6px 12px',
          borderRadius: '20px',
          fontSize: '11px',
          fontWeight: 500,
          background: 'rgba(11, 14, 17, 0.85)',
          border: '1px solid rgba(240, 185, 11, 0.2)',
          color: '#5E6673',
          backdropFilter: 'blur(8px)',
          textDecoration: 'none',
          transition: 'all 0.2s ease',
          letterSpacing: '0.3px',
        }}
        onMouseEnter={e => {
          const el = e.currentTarget
          el.style.color = '#F0B90B'
          el.style.borderColor = 'rgba(240, 185, 11, 0.5)'
          el.style.boxShadow = '0 0 12px rgba(240, 185, 11, 0.15)'
        }}
        onMouseLeave={e => {
          const el = e.currentTarget
          el.style.color = '#5E6673'
          el.style.borderColor = 'rgba(240, 185, 11, 0.2)'
          el.style.boxShadow = 'none'
        }}
      >
        <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor">
          <path d="M12 2C6.477 2 2 6.477 2 12c0 4.418 2.865 8.166 6.839 9.489.5.092.682-.217.682-.482 0-.237-.008-.866-.013-1.7-2.782.604-3.369-1.34-3.369-1.34-.454-1.156-1.11-1.463-1.11-1.463-.908-.62.069-.608.069-.608 1.003.07 1.531 1.03 1.531 1.03.892 1.529 2.341 1.087 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.11-4.555-4.943 0-1.091.39-1.984 1.029-2.683-.103-.253-.446-1.27.098-2.647 0 0 .84-.269 2.75 1.025A9.578 9.578 0 0 1 12 6.836c.85.004 1.705.114 2.504.336 1.909-1.294 2.747-1.025 2.747-1.025.546 1.377.203 2.394.1 2.647.64.699 1.028 1.592 1.028 2.683 0 3.842-2.339 4.687-4.566 4.935.359.309.678.919.678 1.852 0 1.336-.012 2.415-.012 2.743 0 .267.18.578.688.48C19.138 20.163 22 16.418 22 12c0-5.523-4.477-10-10-10z"/>
        </svg>
        Built by Hijra Muksin · IZF
      </a>
    </>
  )
}
