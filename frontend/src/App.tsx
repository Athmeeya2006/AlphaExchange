import React from 'react';
import { BrowserRouter, Routes, Route, Link, useLocation } from 'react-router-dom';

const LeaderboardPage = () => (
  <div className="p-8">
    <h1 className="text-2xl font-bold mb-4 font-mono text-accent-green">Live Leaderboard</h1>
    <div className="bg-surface-secondary p-6 rounded-lg border border-surface-tertiary">
      <p className="text-gray-400 font-mono">No data yet...</p>
    </div>
  </div>
);

const SubmitPage = () => (
  <div className="p-8">
    <h1 className="text-2xl font-bold mb-4 font-mono text-accent-green">Submit Your Order Book</h1>
    <div className="bg-surface-secondary p-6 rounded-lg border border-surface-tertiary">
      <p className="text-gray-400 font-mono">Submission form coming soon...</p>
    </div>
  </div>
);

const LoginPage = () => (
  <div className="p-8">
    <h1 className="text-2xl font-bold mb-4 font-mono text-accent-green">Enter API Key</h1>
    <div className="bg-surface-secondary p-6 rounded-lg border border-surface-tertiary max-w-md">
      <input 
        type="password" 
        placeholder="API Key..." 
        className="w-full bg-surface-primary border border-surface-hover rounded p-2 text-gray-200 font-mono focus:outline-none focus:border-accent-green"
      />
      <button className="mt-4 bg-accent-green text-surface-primary font-bold py-2 px-4 rounded hover:bg-opacity-90 font-mono transition-colors">
        Login
      </button>
    </div>
  </div>
);

const NavLink = ({ to, children }: { to: string; children: React.ReactNode }) => {
  const location = useLocation();
  const isActive = location.pathname === to;
  
  return (
    <Link 
      to={to} 
      className={`font-mono px-4 py-2 rounded transition-colors ${
        isActive ? 'bg-surface-hover text-accent-green' : 'text-gray-400 hover:text-gray-200 hover:bg-surface-tertiary'
      }`}
    >
      {children}
    </Link>
  );
};

const NavBar = () => (
  <nav className="bg-surface-secondary border-b border-surface-tertiary p-4 flex gap-4 items-center">
    <div className="font-bold font-mono text-xl mr-8 flex items-center">
      <span className="text-accent-green mr-2">█</span> AlphaExchange
    </div>
    <NavLink to="/">Leaderboard</NavLink>
    <NavLink to="/submit">Submit</NavLink>
    <NavLink to="/login">Login</NavLink>
  </nav>
);

function App() {
  return (
    <BrowserRouter>
      <div className="min-h-screen flex flex-col">
        <NavBar />
        <main className="flex-1">
          <Routes>
            <Route path="/" element={<LeaderboardPage />} />
            <Route path="/submit" element={<SubmitPage />} />
            <Route path="/login" element={<LoginPage />} />
          </Routes>
        </main>
      </div>
    </BrowserRouter>
  );
}

export default App;
