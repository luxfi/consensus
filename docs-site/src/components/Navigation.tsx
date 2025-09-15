import { Link } from 'react-router-dom'
import { Menu, X, Github, Book, Code2 } from 'lucide-react'
import { useState } from 'react'
import LuxLogo from './LuxLogo'

export default function Navigation() {
  const [isOpen, setIsOpen] = useState(false)

  return (
    <nav className="border-b border-gray-200 sticky top-0 z-50 bg-white/95 backdrop-blur-lg">
      <div className="container">
        <div className="flex h-16 items-center justify-between">
          {/* Logo */}
          <LuxLogo 
            href="/" 
            size="md"
            variant="full"
            outerClx="no-underline"
            textClx="text-black"
          />

          {/* Desktop Nav */}
          <div className="hidden md:flex items-center space-x-8">
            <Link to="/docs" className="flex items-center space-x-1 text-gray-600 hover:text-black transition-colors">
              <Book className="w-4 h-4" />
              <span className="font-medium">Documentation</span>
            </Link>
            <Link to="/docs#languages" className="flex items-center space-x-1 text-gray-600 hover:text-black transition-colors">
              <Code2 className="w-4 h-4" />
              <span className="font-medium">Languages</span>
            </Link>
            <a 
              href="https://github.com/luxfi/consensus" 
              target="_blank" 
              rel="noopener noreferrer"
              className="flex items-center space-x-1 text-gray-600 hover:text-black transition-colors"
            >
              <Github className="w-4 h-4" />
              <span className="font-medium">GitHub</span>
            </a>
          </div>

          {/* Mobile menu button */}
          <button
            onClick={() => setIsOpen(!isOpen)}
            className="md:hidden p-2"
          >
            {isOpen ? <X className="w-6 h-6" /> : <Menu className="w-6 h-6" />}
          </button>
        </div>

        {/* Mobile Nav */}
        {isOpen && (
          <div className="md:hidden py-4 border-t border-gray-200">
            <div className="flex flex-col space-y-4">
              <Link 
                to="/docs" 
                className="flex items-center space-x-2 py-2 text-gray-600 hover:text-black transition-colors"
                onClick={() => setIsOpen(false)}
              >
                <Book className="w-4 h-4" />
                <span className="font-medium">Documentation</span>
              </Link>
              <Link 
                to="/docs#languages" 
                className="flex items-center space-x-2 py-2 text-gray-600 hover:text-black transition-colors"
                onClick={() => setIsOpen(false)}
              >
                <Code2 className="w-4 h-4" />
                <span className="font-medium">Languages</span>
              </Link>
              <a 
                href="https://github.com/luxfi/consensus" 
                target="_blank" 
                rel="noopener noreferrer"
                className="flex items-center space-x-2 py-2 text-gray-600 hover:text-black transition-colors"
              >
                <Github className="w-4 h-4" />
                <span className="font-medium">GitHub</span>
              </a>
            </div>
          </div>
        )}
      </div>
    </nav>
  )
}