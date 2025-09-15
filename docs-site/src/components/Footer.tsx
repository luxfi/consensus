import { LuxIcon } from './LuxLogo'

export default function Footer() {
  return (
    <footer className="border-t border-gray-200 py-12 mt-24 bg-white">
      <div className="container">
        <div className="flex flex-col md:flex-row justify-between items-center space-y-4 md:space-y-0">
          <div className="flex items-center space-x-3">
            <LuxIcon className="w-6 h-6 text-gray-400" />
            <span className="font-mono text-sm text-gray-600">
              © 2024 Lux Industries Inc.
            </span>
          </div>
          
          <div className="flex items-center space-x-6">
            <a 
              href="https://lux.network" 
              target="_blank" 
              rel="noopener noreferrer"
              className="text-sm text-gray-600 hover:text-black transition-colors font-medium"
            >
              Lux Network
            </a>
            <a 
              href="https://hanzo.ai" 
              target="_blank" 
              rel="noopener noreferrer"
              className="text-sm text-gray-600 hover:text-black transition-colors font-medium"
            >
              Hanzo AI
            </a>
            <a 
              href="https://github.com/luxfi" 
              target="_blank" 
              rel="noopener noreferrer"
              className="text-sm text-gray-600 hover:text-black transition-colors font-medium"
            >
              GitHub
            </a>
          </div>
        </div>
      </div>
    </footer>
  )
}