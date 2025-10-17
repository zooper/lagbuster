import React, { useEffect, useState } from 'react';
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  TimeScale,
} from 'chart.js';
import { Line } from 'react-chartjs-2';
import 'chartjs-adapter-date-fns';
import { getMetrics } from '../api/client';
import { TimeRange, MetricPoint } from '../types';
import './LatencyGraph.css';

ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  TimeScale
);

interface LatencyGraphProps {
  peerName: string;
  initialRange?: TimeRange;
}

export function LatencyGraph({ peerName, initialRange = '1h' }: LatencyGraphProps) {
  const [range, setRange] = useState<TimeRange>(initialRange);
  const [data, setData] = useState<MetricPoint[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let mounted = true;

    async function fetchData() {
      setLoading(true);
      setError(null);
      try {
        const metrics = await getMetrics(peerName, range);
        if (mounted) {
          setData(metrics.points);
        }
      } catch (err) {
        if (mounted) {
          setError(err instanceof Error ? err.message : 'Failed to load metrics');
        }
      } finally {
        if (mounted) {
          setLoading(false);
        }
      }
    }

    fetchData();
    return () => {
      mounted = false;
    };
  }, [peerName, range]);

  const chartData = {
    labels: data.map((p) => new Date(p.timestamp)),
    datasets: [
      {
        label: 'Latency (ms)',
        data: data.map((p) => p.latency),
        borderColor: 'rgb(75, 192, 192)',
        backgroundColor: 'rgba(75, 192, 192, 0.2)',
        borderWidth: 2,
        pointRadius: data.length > 100 ? 0 : 3,
        pointBackgroundColor: (context: any) => {
          const point = data[context.dataIndex];
          return point?.is_healthy ? 'rgb(75, 192, 192)' : 'rgb(255, 99, 132)';
        },
        segment: {
          borderColor: (context: any) => {
            const point = data[context.p0DataIndex];
            return point?.is_healthy ? 'rgb(75, 192, 192)' : 'rgb(255, 99, 132)';
          },
        },
      },
    ],
  };

  const options: any = {
    responsive: true,
    maintainAspectRatio: false,
    plugins: {
      legend: {
        display: true,
        position: 'top' as const,
      },
      title: {
        display: true,
        text: `${peerName} - Latency Over Time`,
      },
    },
    scales: {
      x: {
        type: 'time' as const,
        time: {
          unit: range === '1h' ? 'minute' : range === '24h' ? 'hour' : 'day',
        },
        title: {
          display: true,
          text: 'Time',
        },
      },
      y: {
        title: {
          display: true,
          text: 'Latency (ms)',
        },
        beginAtZero: true,
      },
    },
  };

  return (
    <div className="latency-graph">
      <div className="graph-controls">
        <div className="range-selector">
          {(['1h', '24h', '7d', '30d'] as TimeRange[]).map((r) => (
            <button
              key={r}
              className={range === r ? 'active' : ''}
              onClick={() => setRange(r)}
            >
              {r}
            </button>
          ))}
        </div>
      </div>

      <div className="graph-container">
        {loading && <div className="loading">Loading metrics...</div>}
        {error && <div className="error">Error: {error}</div>}
        {!loading && !error && data.length === 0 && (
          <div className="no-data">No data available for this time range</div>
        )}
        {!loading && !error && data.length > 0 && (
          <Line data={chartData} options={options} />
        )}
      </div>
    </div>
  );
}
