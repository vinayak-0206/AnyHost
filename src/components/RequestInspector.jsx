import { useEffect, useState } from 'react';
import { api } from '../api';

export default function RequestInspector({ token, subdomain, onClose }) {
  const [logs, setLogs] = useState([]);
  const [selectedLog, setSelectedLog] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [autoRefresh, setAutoRefresh] = useState(true);

  const fetchLogs = async () => {
    try {
      const data = await api.getRequestLogs(token, subdomain);
      setLogs(data.logs || []);
      setError(null);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchLogs();

    let interval;
    if (autoRefresh) {
      interval = setInterval(fetchLogs, 3000);
    }

    return () => {
      if (interval) clearInterval(interval);
    };
  }, [token, subdomain, autoRefresh]);

  const formatTime = (timestamp) => {
    return new Date(timestamp).toLocaleTimeString();
  };

  const getStatusColor = (status) => {
    if (status >= 200 && status < 300) return 'bg-green-100 text-green-800';
    if (status >= 300 && status < 400) return 'bg-blue-100 text-blue-800';
    if (status >= 400 && status < 500) return 'bg-yellow-100 text-yellow-800';
    if (status >= 500) return 'bg-red-100 text-red-800';
    return 'bg-gray-100 text-gray-800';
  };

  const getMethodColor = (method) => {
    const colors = {
      GET: 'bg-blue-500',
      POST: 'bg-green-500',
      PUT: 'bg-yellow-500',
      PATCH: 'bg-orange-500',
      DELETE: 'bg-red-500',
    };
    return colors[method] || 'bg-gray-500';
  };

  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
      <div className="bg-white rounded-lg shadow-xl w-full max-w-6xl h-[80vh] flex flex-col">
        {/* Header */}
        <div className="flex items-center justify-between p-4 border-b">
          <div className="flex items-center gap-4">
            <h2 className="text-xl font-bold">Request Inspector</h2>
            <span className="text-gray-500">{subdomain}</span>
          </div>
          <div className="flex items-center gap-4">
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={autoRefresh}
                onChange={(e) => setAutoRefresh(e.target.checked)}
                className="rounded"
              />
              Auto-refresh
            </label>
            <button
              onClick={fetchLogs}
              className="px-3 py-1 text-sm bg-gray-100 rounded hover:bg-gray-200"
            >
              Refresh
            </button>
            <button
              onClick={onClose}
              className="text-gray-500 hover:text-gray-700 text-2xl"
            >
              x
            </button>
          </div>
        </div>

        {/* Content */}
        <div className="flex flex-1 overflow-hidden">
          {/* Request List */}
          <div className="w-1/2 border-r overflow-y-auto">
            {loading && (
              <div className="p-4 text-center text-gray-500">Loading...</div>
            )}
            {error && (
              <div className="p-4 text-center text-red-500">{error}</div>
            )}
            {!loading && logs.length === 0 && (
              <div className="p-4 text-center text-gray-500">
                No requests yet. Requests to {subdomain}.* will appear here.
              </div>
            )}
            {logs.map((log) => (
              <div
                key={log.id}
                onClick={() => setSelectedLog(log)}
                className={`p-3 border-b cursor-pointer hover:bg-gray-50 ${
                  selectedLog?.id === log.id ? 'bg-blue-50' : ''
                }`}
              >
                <div className="flex items-center gap-3">
                  <span
                    className={`px-2 py-0.5 text-xs text-white rounded ${getMethodColor(
                      log.method
                    )}`}
                  >
                    {log.method}
                  </span>
                  <span className="flex-1 font-mono text-sm truncate">
                    {log.path}
                  </span>
                  <span
                    className={`px-2 py-0.5 text-xs rounded ${getStatusColor(
                      log.status_code
                    )}`}
                  >
                    {log.status_code}
                  </span>
                </div>
                <div className="flex items-center gap-3 mt-1 text-xs text-gray-500">
                  <span>{formatTime(log.created_at)}</span>
                  <span>{log.duration_ms}ms</span>
                  {log.client_ip && <span>{log.client_ip}</span>}
                </div>
              </div>
            ))}
          </div>

          {/* Request Details */}
          <div className="w-1/2 overflow-y-auto">
            {selectedLog ? (
              <div className="p-4">
                <div className="mb-4">
                  <div className="flex items-center gap-3 mb-2">
                    <span
                      className={`px-2 py-1 text-sm text-white rounded ${getMethodColor(
                        selectedLog.method
                      )}`}
                    >
                      {selectedLog.method}
                    </span>
                    <span
                      className={`px-2 py-1 text-sm rounded ${getStatusColor(
                        selectedLog.status_code
                      )}`}
                    >
                      {selectedLog.status_code}
                    </span>
                    <span className="text-sm text-gray-500">
                      {selectedLog.duration_ms}ms
                    </span>
                  </div>
                  <div className="font-mono text-sm break-all bg-gray-100 p-2 rounded">
                    {selectedLog.path}
                  </div>
                </div>

                {/* Request Headers */}
                {selectedLog.request_headers && (
                  <div className="mb-4">
                    <h3 className="font-semibold mb-2 text-sm">
                      Request Headers
                    </h3>
                    <pre className="bg-gray-900 text-green-400 p-3 rounded text-xs overflow-x-auto">
                      {selectedLog.request_headers}
                    </pre>
                  </div>
                )}

                {/* Request Body */}
                {selectedLog.request_body && (
                  <div className="mb-4">
                    <h3 className="font-semibold mb-2 text-sm">Request Body</h3>
                    <pre className="bg-gray-900 text-green-400 p-3 rounded text-xs overflow-x-auto max-h-48">
                      {tryFormatJson(selectedLog.request_body)}
                    </pre>
                  </div>
                )}

                {/* Response Headers */}
                {selectedLog.response_headers && (
                  <div className="mb-4">
                    <h3 className="font-semibold mb-2 text-sm">
                      Response Headers
                    </h3>
                    <pre className="bg-gray-900 text-blue-400 p-3 rounded text-xs overflow-x-auto">
                      {selectedLog.response_headers}
                    </pre>
                  </div>
                )}

                {/* Response Body */}
                {selectedLog.response_body && (
                  <div className="mb-4">
                    <h3 className="font-semibold mb-2 text-sm">Response Body</h3>
                    <pre className="bg-gray-900 text-blue-400 p-3 rounded text-xs overflow-x-auto max-h-48">
                      {tryFormatJson(selectedLog.response_body)}
                    </pre>
                  </div>
                )}

                {/* Metadata */}
                <div className="text-xs text-gray-500 mt-4 pt-4 border-t">
                  <div>Time: {new Date(selectedLog.created_at).toLocaleString()}</div>
                  {selectedLog.client_ip && (
                    <div>Client IP: {selectedLog.client_ip}</div>
                  )}
                  {selectedLog.user_agent && (
                    <div>User Agent: {selectedLog.user_agent}</div>
                  )}
                </div>
              </div>
            ) : (
              <div className="flex items-center justify-center h-full text-gray-500">
                Select a request to view details
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

// Helper to format JSON if possible
function tryFormatJson(str) {
  try {
    const parsed = JSON.parse(str);
    return JSON.stringify(parsed, null, 2);
  } catch {
    return str;
  }
}
