import { useEffect, useState } from 'react';
import { api } from '../api';
import RequestInspector from '../components/RequestInspector';

export default function Dashboard({ token }) {
  const [tunnels, setTunnels] = useState([]);
  const [newSub, setNewSub] = useState('');
  const [refreshKey, setRefreshKey] = useState(0);
  const [copied, setCopied] = useState(null);
  const [inspectorSubdomain, setInspectorSubdomain] = useState(null);

  // Generate the server WebSocket URL from current location
  const getServerURL = () => {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    return `${proto}//${window.location.host}/tunnel`;
  };

  // Generate the full CLI command
  const getCommand = (subdomain) => {
    return `./bin/gotunnel --server ${getServerURL()} --token ${token} --subdomain ${subdomain} --port 3000`;
  };

  const copyCommand = (subdomain) => {
    navigator.clipboard.writeText(getCommand(subdomain));
    setCopied(subdomain);
    setTimeout(() => setCopied(null), 2000);
  };

  useEffect(() => {
    let cancelled = false;
    api.getTunnels(token).then(data => {
      if (!cancelled) {
        setTunnels(data || []);
      }
    });
    return () => { cancelled = true; };
  }, [token, refreshKey]);

  const handleReserve = async (e) => {
    e.preventDefault();
    await api.reserveTunnel(token, newSub);
    setNewSub('');
    setRefreshKey(k => k + 1);
  };

  return (
    <div className="p-8 max-w-4xl mx-auto">
      <h1 className="text-2xl font-bold mb-6">Your Tunnels</h1>
      
      {/* Reserve Form */}
      <div className="bg-white p-6 rounded shadow mb-8">
        <h2 className="text-lg font-semibold mb-4">Reserve New Subdomain</h2>
        <form onSubmit={handleReserve} className="flex gap-4">
          <input 
            value={newSub}
            onChange={e => setNewSub(e.target.value)}
            placeholder="subdomain"
            className="border p-2 rounded flex-1"
          />
          <button className="bg-blue-600 text-white px-4 py-2 rounded">
            Reserve
          </button>
        </form>
      </div>

      {/* List */}
      <div className="grid gap-4">
        {tunnels.map(t => (
          <div key={t.subdomain} className="bg-white p-4 rounded shadow">
            <div className="flex justify-between items-center mb-3">
              <div>
                <div className="font-bold text-lg">{t.subdomain}</div>
                <div className="text-gray-500 text-sm">Public URL: {window.location.origin}/{t.subdomain}/</div>
              </div>
              <span className={`px-2 py-1 rounded text-sm ${t.status === 'online' ? 'bg-green-100 text-green-800' : 'bg-gray-100 text-gray-800'}`}>
                {t.status || 'offline'}
              </span>
            </div>

            {/* Command box */}
            <div className="bg-gray-900 text-green-400 p-3 rounded font-mono text-sm overflow-x-auto">
              {getCommand(t.subdomain)}
            </div>

            <div className="flex gap-2 mt-2">
              <button
                className="bg-blue-600 text-white px-4 py-2 rounded text-sm hover:bg-blue-700"
                onClick={() => copyCommand(t.subdomain)}
              >
                {copied === t.subdomain ? 'Copied!' : 'Copy Command'}
              </button>
              <button
                className="bg-purple-600 text-white px-4 py-2 rounded text-sm hover:bg-purple-700"
                onClick={() => setInspectorSubdomain(t.subdomain)}
              >
                Inspect Requests
              </button>
            </div>
          </div>
        ))}
      </div>

      {tunnels.length === 0 && (
        <div className="text-center text-gray-500 py-8">
          No subdomains reserved yet. Reserve one above to get started.
        </div>
      )}

      {/* Request Inspector Modal */}
      {inspectorSubdomain && (
        <RequestInspector
          token={token}
          subdomain={inspectorSubdomain}
          onClose={() => setInspectorSubdomain(null)}
        />
      )}
    </div>
  );
}